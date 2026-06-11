#!/usr/bin/env bash
#
# Emergency Callback — one-shot installer (app server side).
#
#   sudo ./install.sh            # interactive, sensible defaults, auto-generated secrets
#   sudo ./install.sh --yes      # zero-touch: accept every default, generate everything
#
# What it does on THIS (app) server:
#   - ensures Go + PostgreSQL (auto-installs on Debian/Ubuntu if missing)
#   - creates the DB role + database + GRANTs schema privileges (PG15+ safe)
#   - generates secrets (session/CSRF), DB password, admin password, AMI secret
#   - writes .env, builds the binary, runs goose + River migrations
#   - creates the admin user
#   - installs & starts systemd services (web + worker)
#   - saves all credentials to INSTALL_CREDENTIALS.txt (chmod 600)
#
# FreePBX is a SEPARATE server: this script does NOT touch it. Instead it writes
# a ready-to-apply bundle to ./freepbx-bundle/ (AMI user, dialplan, audio files,
# step-by-step README) for you to apply on your FreePBX (GUI or terminal).
#
set -euo pipefail

# ----------------------------------------------------------------------------
# Helpers
# ----------------------------------------------------------------------------
RED=$'\033[0;31m'; GRN=$'\033[0;32m'; YLW=$'\033[1;33m'; BLU=$'\033[0;34m'; BLD=$'\033[1m'; NC=$'\033[0m'
info() { echo "${BLU}==>${NC} $*"; }
ok()   { echo "${GRN} ✓${NC} $*"; }
warn() { echo "${YLW} !${NC} $*"; }
err()  { echo "${RED} ✗${NC} $*" >&2; }
die()  { err "$*"; exit 1; }

ASSUME_YES=0
for a in "$@"; do
  case "$a" in
    --yes|-y) ASSUME_YES=1 ;;
    -h|--help)
      awk 'NR>1 && /^#/{sub(/^# ?/,"");print} NR>1 && !/^#/{exit}' "$0"; exit 0 ;;
    *) die "unknown argument: $a (use --yes or --help)" ;;
  esac
done

# ask VAR "Prompt" "default" [secret]
ask() {
  local __var="$1" __prompt="$2" __default="${3-}" __secret="${4-}" __ans=""
  # env override wins
  if [ -n "${!__var-}" ]; then printf -v "$__var" '%s' "${!__var}"; return; fi
  if [ "$ASSUME_YES" = "1" ] || [ ! -t 0 ]; then printf -v "$__var" '%s' "$__default"; return; fi
  if [ "$__secret" = "secret" ]; then
    read -r -s -p "$__prompt [$__default]: " __ans; echo
  else
    read -r -p "$__prompt [$__default]: " __ans
  fi
  printf -v "$__var" '%s' "${__ans:-$__default}"
}

ask_yn() { # ask_yn VAR "Prompt" default(y/n)
  local __var="$1" __prompt="$2" __def="$3" __ans=""
  if [ "$ASSUME_YES" = "1" ] || [ ! -t 0 ]; then printf -v "$__var" '%s' "$__def"; return; fi
  read -r -p "$__prompt [$( [ "$__def" = y ] && echo 'Y/n' || echo 'y/N')]: " __ans
  __ans="${__ans:-$__def}"; case "$__ans" in [Yy]*) printf -v "$__var" y ;; *) printf -v "$__var" n ;; esac
}

gen_secret() { openssl rand -base64 "${1:-32}" | tr -d '\n/+=' | cut -c1-"${2:-32}"; }
gen_b64()    { openssl rand -base64 "${1:-32}" | tr -d '\n'; }

port_busy() { ss -ltnH "( sport = :$1 )" 2>/dev/null | grep -q . ; }
free_port() { local p="$1"; while port_busy "$p"; do p=$((p+1)); done; echo "$p"; }

# ----------------------------------------------------------------------------
# 0. Root + paths
# ----------------------------------------------------------------------------
[ "$(id -u)" = "0" ] || die "Run as root (sudo ./install.sh) — needed for apt, PostgreSQL, and systemd."
APP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$APP_DIR"
[ -d cmd/emergency-callback ] || die "Run this from the repository root (cmd/emergency-callback not found)."
SERVICE_USER="$(stat -c %U "$APP_DIR")"; [ "$SERVICE_USER" = "root" ] && SERVICE_USER="root"

echo "${BLD}Emergency Callback installer${NC}"
echo "App dir:      $APP_DIR"
echo "Service user: $SERVICE_USER"
echo

# ----------------------------------------------------------------------------
# 1. System dependencies
# ----------------------------------------------------------------------------
APT=""; command -v apt-get >/dev/null 2>&1 && APT=1

ensure_go() {
  if command -v go >/dev/null 2>&1; then ok "Go present ($(go version | awk '{print $3}'))"; return; fi
  [ -x /usr/local/go/bin/go ] && { export PATH="$PATH:/usr/local/go/bin"; ok "Go present (/usr/local/go)"; return; }
  [ -n "$APT" ] || die "Go missing and not Debian/Ubuntu — install Go 1.23+ manually."
  info "Installing Go…"
  local ver="1.23.0" arch; arch="$(dpkg --print-architecture)"
  curl -fsSL "https://go.dev/dl/go${ver}.linux-${arch}.tar.gz" -o /tmp/go.tgz
  rm -rf /usr/local/go && tar -C /usr/local -xzf /tmp/go.tgz
  echo 'export PATH=$PATH:/usr/local/go/bin' > /etc/profile.d/go.sh
  export PATH="$PATH:/usr/local/go/bin"; ok "Go installed ($ver)"
}

ensure_pg() {
  if command -v psql >/dev/null 2>&1 && (sudo -u postgres psql -tAc 'SELECT 1' >/dev/null 2>&1); then
    ok "PostgreSQL present"; return
  fi
  [ -n "$APT" ] || die "PostgreSQL missing and not Debian/Ubuntu — install PostgreSQL 14+ manually."
  info "Installing PostgreSQL…"
  apt-get update -qq && apt-get install -y -qq postgresql >/dev/null
  systemctl enable --now postgresql >/dev/null 2>&1 || true
  ok "PostgreSQL installed"
}

ensure_river() {
  if command -v river >/dev/null 2>&1; then ok "River CLI present"; return; fi
  [ -x /usr/local/bin/river ] && { ok "River CLI present"; return; }
  info "Installing River CLI…"
  GOBIN=/usr/local/bin GOFLAGS=-mod=mod go install github.com/riverqueue/river/cmd/river@latest
  ok "River CLI installed (/usr/local/bin/river)"
}

info "Checking prerequisites…"
command -v curl >/dev/null 2>&1 || { [ -n "$APT" ] && apt-get install -y -qq curl >/dev/null; }
command -v openssl >/dev/null 2>&1 || { [ -n "$APT" ] && apt-get install -y -qq openssl >/dev/null; }
ensure_go
ensure_pg
ensure_river
echo

# ----------------------------------------------------------------------------
# 2. Gather configuration
# ----------------------------------------------------------------------------
info "Configuration (press Enter to accept defaults)…"
SERVER_IP="$(hostname -I 2>/dev/null | awk '{print $1}')"; SERVER_IP="${SERVER_IP:-127.0.0.1}"

ask DB_NAME      "Database name"        "emergency_callback"
ask DB_USER      "Database user"        "ecb"
ask DB_HOST      "Database host"        "127.0.0.1"
ask DB_PORT      "Database port"        "5432"
DB_PASSWORD="${DB_PASSWORD:-$(gen_secret 24 24)}"   # generated unless pre-set

ask HTTP_PORT    "Web port"             "8000"
HTTP_PORT="${HTTP_PORT#:}"
if port_busy "$HTTP_PORT"; then
  NEWP="$(free_port "$((HTTP_PORT+1))")"
  warn "Port $HTTP_PORT is busy → using $NEWP"
  HTTP_PORT="$NEWP"
fi
ask SITE_DOMAIN  "Public site URL (for SMS links)" "http://${SERVER_IP}:${HTTP_PORT}"

ask ADMIN_USER   "Admin username"       "admin"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-$(gen_secret 16 16)}"

# --- AMI (FreePBX is remote) ---
ask_yn HAVE_AMI "Do you already have an AMI user on FreePBX?" n
ask AMI_HOST "FreePBX AMI host (IP of your FreePBX)" "127.0.0.1"
ask AMI_PORT "FreePBX AMI port" "5038"
if [ "$HAVE_AMI" = "y" ]; then
  ask AMI_USERNAME "Existing AMI username" "ecb"
  ask AMI_SECRET   "Existing AMI secret"   ""  secret
  [ -n "$AMI_SECRET" ] || die "AMI secret required when using an existing AMI user."
  AMI_IS_NEW=0
else
  ask AMI_USERNAME "New AMI username (will be created on FreePBX)" "ecb"
  AMI_SECRET="${AMI_SECRET:-$(gen_secret 24 24)}"   # generated
  AMI_IS_NEW=1
fi
ask AMI_CALLER_ID "Caller ID number (shown to the callee)" "103"

# --- Eskiz SMS ---
ask ESKIZ_EMAIL    "Eskiz email (blank = SMS disabled / dry-run)" ""
ESKIZ_DRY_RUN="false"
if [ -z "$ESKIZ_EMAIL" ]; then ESKIZ_DRY_RUN="true"; ESKIZ_PASSWORD=""; else
  ask ESKIZ_PASSWORD "Eskiz password" "" secret
fi

DATABASE_URL="postgres://${DB_USER}:${DB_PASSWORD}@${DB_HOST}:${DB_PORT}/${DB_NAME}?sslmode=disable"
SESSION_SECRET="$(gen_b64 32)"
CSRF_KEY="$(gen_secret 32 32)"
echo
ok "Configuration collected"
echo

# ----------------------------------------------------------------------------
# 3. PostgreSQL: role + database + grants (idempotent, PG15+ safe)
# ----------------------------------------------------------------------------
info "Setting up PostgreSQL (db=$DB_NAME user=$DB_USER)…"
psql_super() { sudo -u postgres psql -v ON_ERROR_STOP=1 "$@"; }
ESC_PW="${DB_PASSWORD//\'/\'\'}"
if psql_super -tAc "SELECT 1 FROM pg_roles WHERE rolname='${DB_USER}'" | grep -q 1; then
  psql_super -c "ALTER ROLE \"${DB_USER}\" LOGIN PASSWORD '${ESC_PW}'" >/dev/null
  ok "Role ${DB_USER} exists (password synced)"
else
  psql_super -c "CREATE ROLE \"${DB_USER}\" LOGIN PASSWORD '${ESC_PW}'" >/dev/null
  ok "Role ${DB_USER} created"
fi
if psql_super -tAc "SELECT 1 FROM pg_database WHERE datname='${DB_NAME}'" | grep -q 1; then
  ok "Database ${DB_NAME} exists"
else
  psql_super -c "CREATE DATABASE \"${DB_NAME}\" OWNER \"${DB_USER}\"" >/dev/null
  ok "Database ${DB_NAME} created"
fi
psql_super -d "${DB_NAME}" -c "GRANT ALL ON SCHEMA public TO \"${DB_USER}\"" >/dev/null
psql_super -d "${DB_NAME}" -c "ALTER SCHEMA public OWNER TO \"${DB_USER}\"" >/dev/null 2>&1 || true
ok "Schema privileges granted (PG15+ safe)"
echo

# ----------------------------------------------------------------------------
# 4. Write .env
# ----------------------------------------------------------------------------
info "Writing .env…"
[ -f .env ] && cp -a .env ".env.bak.$(date +%s)" && warn "Existing .env backed up"
cat > .env <<ENV
# Generated by install.sh on $(date -u +%Y-%m-%dT%H:%M:%SZ)
# Database
DATABASE_URL=${DATABASE_URL}
DB_POOL_MAX_CONNS=10
DB_POOL_MIN_CONNS=2

# Server
HTTP_ADDR=:${HTTP_PORT}
SITE_DOMAIN=${SITE_DOMAIN}
SESSION_SECRET=${SESSION_SECRET}
CSRF_KEY=${CSRF_KEY}

# Asterisk AMI (FreePBX is a separate server)
AMI_HOST=${AMI_HOST}
AMI_PORT=${AMI_PORT}
AMI_USERNAME=${AMI_USERNAME}
AMI_SECRET=${AMI_SECRET}
AMI_CALLER_ID=${AMI_CALLER_ID}
AMI_OPERATOR_QUEUE=777
AMI_CALL_TIMEOUT=60
AMI_RATING_RETRY_LIMIT=3
AMI_RATING_TIMEOUT=10

# Eskiz SMS
ESKIZ_EMAIL=${ESKIZ_EMAIL}
ESKIZ_PASSWORD=${ESKIZ_PASSWORD-}
ESKIZ_BASE_URL=https://notify.eskiz.uz/api
ESKIZ_DRY_RUN=${ESKIZ_DRY_RUN}

# Workers
RIVER_MAX_WORKERS=5
ENV
chown "$SERVICE_USER":"$SERVICE_USER" .env 2>/dev/null || true
chmod 600 .env
ok ".env written"
echo

# ----------------------------------------------------------------------------
# 5. Build
# ----------------------------------------------------------------------------
info "Building binary…"
go build -o emergency-callback ./cmd/emergency-callback
chown "$SERVICE_USER":"$SERVICE_USER" emergency-callback 2>/dev/null || true
ok "Built ./emergency-callback"
echo

# ----------------------------------------------------------------------------
# 6. Migrations (goose + River) — same DATABASE_URL
# ----------------------------------------------------------------------------
info "Running migrations…"
./emergency-callback migrate up
river migrate-up --database-url "${DATABASE_URL}" >/dev/null
ok "Schema + job-queue migrations applied"
echo

# ----------------------------------------------------------------------------
# 7. Admin user (idempotent)
# ----------------------------------------------------------------------------
info "Creating admin user…"
if psql_super -d "${DB_NAME}" -tAc "SELECT 1 FROM users WHERE username='${ADMIN_USER}'" | grep -q 1; then
  warn "User '${ADMIN_USER}' already exists — password NOT changed"
  ADMIN_PASSWORD="(unchanged — existing user)"
else
  ./emergency-callback createuser "${ADMIN_USER}" "${ADMIN_PASSWORD}" admin
  ok "Admin '${ADMIN_USER}' created"
fi
echo

# ----------------------------------------------------------------------------
# 8. FreePBX bundle (apply on your remote FreePBX)
# ----------------------------------------------------------------------------
info "Generating FreePBX bundle…"
BUNDLE="$APP_DIR/freepbx-bundle"
rm -rf "$BUNDLE"; mkdir -p "$BUNDLE/sounds"
cp -f audios/ambulance-*.wav "$BUNDLE/sounds/" 2>/dev/null || warn "audios/ not found — add the 6 WAV prompts manually"

cat > "$BUNDLE/manager_custom.conf" <<MGR
; Append to /etc/asterisk/manager_custom.conf on the FreePBX server, then:
;   sudo fwconsole reload   (or: sudo asterisk -rx 'manager reload')
[${AMI_USERNAME}]
secret = ${AMI_SECRET}
deny = 0.0.0.0/0.0.0.0
permit = 0.0.0.0/0.0.0.0
read = system,call,log,verbose,agent,user,config,dtmf,reporting,cdr,dialplan,originate
write = system,call,agent,user,config,command,reporting,originate,message
MGR

cat > "$BUNDLE/extensions_custom.conf" <<'DPLAN'
; Append to /etc/asterisk/extensions_custom.conf on the FreePBX server, then:
;   sudo fwconsole reload   (or: sudo asterisk -rx 'dialplan reload')
; Do NOT add a [from-internal] block — FreePBX owns it (outbound routes).

[ambulance-callback]
exten => s,1,NoOp(ANSWERED CALL_ID=${CALL_ID} PHONE=${PHONE_NUMBER})
 same => n,Answer()
 same => n,UserEvent(CallAnswered,CallID: ${CALL_ID},Phone: ${PHONE_NUMBER})
 same => n,Wait(300)
 same => n,Hangup()

[play-audio]
exten => _.,1,NoOp(PLAY ${EXTEN} CALL_ID=${CALL_ID})
 same => n,Playback(${EXTEN})
 same => n,UserEvent(AudioPlayed,CallID: ${CALL_ID},Audio: ${EXTEN})
 same => n,WaitExten(60)
 same => n,Wait(60)
 same => n,Hangup()

[transfer-to-337]
exten => s,1,NoOp(TRANSFER CALL_ID=${CALL_ID})
 same => n,Dial(Local/337@from-internal,30)   ; <-- set your operator extension/queue
 same => n,Hangup()
DPLAN

cat > "$BUNDLE/README.md" <<RME
# FreePBX setup bundle

Apply these on your **FreePBX server** (it is separate from the app server).

## 1. AMI user
- GUI: Settings → Asterisk Manager Users → Add Manager. Name \`${AMI_USERNAME}\`,
  Secret \`${AMI_SECRET}\`, Read perms incl. **dtmf**, Write perms incl. originate.
- OR terminal: append \`manager_custom.conf\` (this folder) to
  \`/etc/asterisk/manager_custom.conf\`, then \`sudo fwconsole reload\`.

> The app's .env already has AMI_USERNAME=\`${AMI_USERNAME}\` and the matching secret.

## 2. Dialplan
Append \`extensions_custom.conf\` (this folder) to
\`/etc/asterisk/extensions_custom.conf\`, then \`sudo fwconsole reload\`.
Edit the \`transfer-to-337\` Dial() target to your operator extension/queue.

## 3. Audio prompts
Copy \`sounds/ambulance-*.wav\` to your Asterisk sounds dir (e.g.
\`/var/lib/asterisk/sounds/en/\`), then:
\`sudo chown asterisk:asterisk /var/lib/asterisk/sounds/en/ambulance-*.wav\`

## 4. Outbound routing
Ensure an Outbound Route matches the dialed number. The app strips a leading
\`998\` and dials the 9-digit number into \`from-internal\`; add a route whose
dial pattern matches it (prepend \`998\` if your trunk needs it).

Full details: docs site → Telephony → FreePBX Integration.
RME
ok "Bundle at $BUNDLE/"
echo

# ----------------------------------------------------------------------------
# 9. systemd services
# ----------------------------------------------------------------------------
info "Installing systemd services (user=$SERVICE_USER)…"
write_unit() { # write_unit name mode afterunits
  cat > "/etc/systemd/system/emergency-callback-$1.service" <<UNIT
[Unit]
Description=Emergency Callback ($1)
After=network.target postgresql.service $3

[Service]
Type=simple
User=${SERVICE_USER}
WorkingDirectory=${APP_DIR}
ExecStart=${APP_DIR}/emergency-callback $2
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
UNIT
}
write_unit web web ""
write_unit worker worker ""
systemctl daemon-reload
systemctl enable --now emergency-callback-web emergency-callback-worker >/dev/null 2>&1
sleep 2
for s in web worker; do
  if systemctl is-active --quiet "emergency-callback-$s"; then ok "service $s running"; else warn "service $s not active — check: journalctl -u emergency-callback-$s -n 50"; fi
done
echo

# ----------------------------------------------------------------------------
# 10. Credentials file + summary
# ----------------------------------------------------------------------------
CREDS="$APP_DIR/INSTALL_CREDENTIALS.txt"
cat > "$CREDS" <<CR
Emergency Callback — install credentials ($(date -u +%Y-%m-%dT%H:%M:%SZ))
Keep this file safe. chmod 600.

WEB PANEL
  URL:        ${SITE_DOMAIN}
  Local:      http://127.0.0.1:${HTTP_PORT}/users/login/
  Admin user: ${ADMIN_USER}
  Admin pass: ${ADMIN_PASSWORD}

DATABASE
  Name:       ${DB_NAME}
  User:       ${DB_USER}
  Password:   ${DB_PASSWORD}
  URL:        ${DATABASE_URL}

APP SECRETS (in .env)
  SESSION_SECRET: ${SESSION_SECRET}
  CSRF_KEY:       ${CSRF_KEY}

ASTERISK AMI (configure on your FreePBX — see freepbx-bundle/)
  Host:     ${AMI_HOST}:${AMI_PORT}
  Username: ${AMI_USERNAME}
  Secret:   ${AMI_SECRET}
  New user: $( [ "$AMI_IS_NEW" = 1 ] && echo "YES — create it on FreePBX with the secret above" || echo "no — using existing")

ESKIZ SMS
  Email:    ${ESKIZ_EMAIL:-(none — dry-run)}
  Dry-run:  ${ESKIZ_DRY_RUN}

SERVICES
  systemctl status emergency-callback-web
  systemctl status emergency-callback-worker
CR
chown "$SERVICE_USER":"$SERVICE_USER" "$CREDS" 2>/dev/null || true
chmod 600 "$CREDS"

echo "${GRN}${BLD}════════════════════════════════════════════════════════════${NC}"
echo "${GRN}${BLD} Installation complete${NC}"
echo "${GRN}${BLD}════════════════════════════════════════════════════════════${NC}"
echo
echo " ${BLD}Web panel:${NC}  ${SITE_DOMAIN}"
echo " ${BLD}Login:${NC}      ${ADMIN_USER} / ${ADMIN_PASSWORD}"
echo " ${BLD}DB:${NC}         ${DB_NAME} (user ${DB_USER})"
echo " ${BLD}AMI secret:${NC} ${AMI_SECRET}  ($( [ "$AMI_IS_NEW" = 1 ] && echo 'create this AMI user on FreePBX' || echo 'existing'))"
echo
echo " ${BLD}Credentials saved:${NC} ${CREDS}  (chmod 600)"
echo " ${BLD}FreePBX bundle:${NC}    ${BUNDLE}/  — apply on your FreePBX server"
echo
echo " ${YLW}Next:${NC} apply freepbx-bundle/ on FreePBX, then make a test call from the panel."
echo
