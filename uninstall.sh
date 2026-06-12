#!/usr/bin/env bash
#
# Emergency Callback — uninstaller (app server side).
#
#   sudo ./uninstall.sh                 # interactive: stop/remove services, ask about the rest
#   sudo ./uninstall.sh --yes           # remove services + generated files, but KEEP the database
#   sudo ./uninstall.sh --drop-db       # also DROP the database + role (DESTRUCTIVE, data loss)
#   sudo ./uninstall.sh --purge         # everything: services + files + database + role
#
# It does NOT remove system packages (Go, PostgreSQL) or the source code / repo.
# Database name/user are read from .env (DATABASE_URL).
#
set -euo pipefail

RED=$'\033[0;31m'; GRN=$'\033[0;32m'; YLW=$'\033[1;33m'; BLU=$'\033[0;34m'; BLD=$'\033[1m'; NC=$'\033[0m'
info() { echo "${BLU}==>${NC} $*"; }
ok()   { echo "${GRN} ✓${NC} $*"; }
warn() { echo "${YLW} !${NC} $*"; }
die()  { echo "${RED} ✗${NC} $*" >&2; exit 1; }

ASSUME_YES=0; DROP_DB=0
for a in "$@"; do
  case "$a" in
    --yes|-y)   ASSUME_YES=1 ;;
    --drop-db)  DROP_DB=1 ;;
    --purge)    ASSUME_YES=1; DROP_DB=1 ;;
    -h|--help)  awk 'NR>1 && /^#/{sub(/^# ?/,"");print} NR>1 && !/^#/{exit}' "$0"; exit 0 ;;
    *) die "unknown argument: $a" ;;
  esac
done

confirm() { # confirm "Prompt" default(y/n) -> returns 0 for yes
  local p="$1" d="$2" ans=""
  if [ "$ASSUME_YES" = "1" ]; then [ "$d" = y ] && return 0 || return 1; fi
  [ -t 0 ] || { [ "$d" = y ] && return 0 || return 1; }
  read -r -p "$p [$( [ "$d" = y ] && echo 'Y/n' || echo 'y/N')]: " ans; ans="${ans:-$d}"
  case "$ans" in [Yy]*) return 0 ;; *) return 1 ;; esac
}

[ "$(id -u)" = "0" ] || die "Run as root (sudo ./uninstall.sh)."
APP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"; cd "$APP_DIR"

echo "${BLD}Emergency Callback uninstaller${NC}"
echo "App dir: $APP_DIR"
echo

# --- read DB identity from .env (best effort) ---
DB_NAME=""; DB_USER=""
if [ -f .env ]; then
  DB_URL="$(grep -E '^DATABASE_URL=' .env | head -1 | cut -d= -f2-)"
  if [ -n "${DB_URL:-}" ]; then
    # postgres://user:pass@host:port/dbname?params
    DB_USER="$(printf '%s' "$DB_URL" | sed -E 's#^[a-z]+://([^:/?]+).*#\1#')"
    DB_NAME="$(printf '%s' "$DB_URL" | sed -E 's#^.*/([^/?]+)(\?.*)?$#\1#')"
  fi
fi
[ -n "$DB_NAME" ] && info "Detected database: ${BLD}$DB_NAME${NC} (user ${BLD}$DB_USER${NC})" || warn "Could not read DATABASE_URL from .env"
echo

# --- 1. systemd services ---
if confirm "Stop and remove systemd services (web + worker)?" y; then
  for s in web worker; do
    unit="emergency-callback-$s"
    systemctl stop "$unit" >/dev/null 2>&1 || true
    systemctl disable "$unit" >/dev/null 2>&1 || true
    rm -f "/etc/systemd/system/$unit.service"
  done
  systemctl daemon-reload
  ok "systemd services stopped and removed"
else
  warn "Left systemd services in place"
fi
echo

# --- 2. database + role (destructive) ---
if [ -n "$DB_NAME" ]; then
  do_drop=0
  if [ "$DROP_DB" = "1" ]; then do_drop=1
  elif confirm "${RED}DROP database '$DB_NAME' and role '$DB_USER'? This DELETES ALL DATA${NC}" n; then do_drop=1; fi
  if [ "$do_drop" = "1" ]; then
    psql_super() { sudo -u postgres psql -v ON_ERROR_STOP=1 "$@"; }
    info "Terminating connections to $DB_NAME…"
    psql_super -c "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname='${DB_NAME}' AND pid <> pg_backend_pid();" >/dev/null 2>&1 || true
    psql_super -c "DROP DATABASE IF EXISTS \"${DB_NAME}\";" >/dev/null && ok "Database $DB_NAME dropped"
    if [ -n "$DB_USER" ] && [ "$DB_USER" != "postgres" ]; then
      psql_super -c "DROP ROLE IF EXISTS \"${DB_USER}\";" >/dev/null 2>&1 && ok "Role $DB_USER dropped" || warn "Could not drop role $DB_USER (may own other objects)"
    fi
  else
    warn "Kept the database and role"
  fi
else
  warn "Skipping database drop (no DB detected)"
fi
echo

# --- 3. generated files ---
# --yes and --purge both remove generated files; interactive defaults to keeping them.
remove_files=0
if [ "$ASSUME_YES" = "1" ]; then
  remove_files=1
elif confirm "Remove generated files (.env, binary, credentials, freepbx-bundle)?" n; then
  remove_files=1
fi
if [ "$remove_files" = "1" ]; then
  rm -f emergency-callback INSTALL_CREDENTIALS.txt .env .env.bak.*
  rm -rf freepbx-bundle
  ok "Generated files removed"
  warn "Source code and repo are left intact"
else
  warn "Kept generated files"
fi
echo

echo "${GRN}${BLD}Uninstall complete.${NC}"
echo "Note: system packages (Go, PostgreSQL) and the source code were NOT removed."
