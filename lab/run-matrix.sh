#!/usr/bin/env bash
#
# Emergency Callback lab — test matritsa boshqaruvchisi (host'da ishlaydi).
#
#   ./run-matrix.sh build        binarni lab/artifacts/ ga build qilish
#   ./run-matrix.sh up           db+pbx+callee ko'tarish (birinchi marta FreePBX install ~10 daq)
#   ./run-matrix.sh provision    PBX provisioning (extension/AMI/ARI/audio)
#   ./run-matrix.sh s<N>         bitta ssenariy (quyida)
#   ./run-matrix.sh e2e-ami|e2e-ari  end-to-end qo'ng'iroq testi
#   ./run-matrix.sh down         hammasini o'chirish (volumelar saqlanadi)
#   ./run-matrix.sh nuke         hammasi + volumelar
#
# Ssenariylar:
#   s1  toza o'rnatish (app konteyner, systemd, non-interactive setup)
#   s2  re-run idempotentlik (.env bayt-bir xil)
#   s3  band port (setup keyingi portni taklif qiladi)
#   s4  systemd yo'q (app-plain) — graceful, run-web.sh ishlaydi
#   s5  sox yo'q — WARN, o'rnatish davom etadi
#   s6  ARI probe (host'dan doctor uslubida)
#   s7  sounds bor/yo'q — ladder tekshiruvi
#   s8  res_http_media_cache bor/yo'q
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"

C="docker compose"
PBX="$C exec -T pbx"
APP="$C exec -T app"
PLAIN="$C exec -T app-plain"
CALLEE="$C exec -T callee"

info() { echo -e "\n\033[0;34m════ $* ════\033[0m"; }
pass() { echo -e "\033[0;32m ✓ PASS: $*\033[0m"; }
fail() { echo -e "\033[0;31m ✗ FAIL: $*\033[0m"; exit 1; }

build() {
  info "binar build (linux/amd64)"
  mkdir -p artifacts
  (cd .. && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o lab/artifacts/emergency-callback ./cmd/emergency-callback)
  pass "artifacts/emergency-callback"
}

up() {
  info "db + pbx + callee ko'tarilmoqda"
  $C up -d db pbx callee app app-plain
  echo "FreePBX asterisk holati kutilmoqda…"
  for i in $(seq 1 60); do
    if $PBX asterisk -rx 'core show version' >/dev/null 2>&1; then break; fi
    sleep 5; printf .
  done; echo
  if ! $PBX asterisk -rx 'core show version' 2>/dev/null; then
    # Birinchi ishga tushirish: FreePBX hali o'rnatilmagan bo'lishi mumkin
    info "FreePBX o'rnatilmoqda (bir martalik, ~10 daqiqa)…"
    $PBX bash -c 'cd /usr/local/src/freepbx && php install -n --dbuser=freepbxuser --dbpass=labfpbx --dbhost=172.28.0.10' \
      || fail "FreePBX install"
    sleep 5
  fi
  $PBX asterisk -rx 'core show version' || fail "asterisk ishga tushmadi"
  pass "PBX tayyor"
}

provision() {
  info "PBX provisioning"
  $PBX bash /provision-pbx.sh || fail "provision"
  sleep 3
  info "callee registratsiyasi"
  $CALLEE asterisk -rx 'pjsip show registrations' || true
  $PBX asterisk -rx 'pjsip show contacts' | grep -E "200|337" || echo " ! contactlar hali ko'rinmayapti"
  pass "provision tugadi"
}

# app konteynerda umumiy non-interactive setup env
setup_env() {
  cat <<'EOF'
SETUP_APP_DIR=/opt/ecb
SETUP_SERVICE_USER=root
SETUP_PG_MODE=install
SETUP_DB_NAME=emergency_callback
SETUP_DB_USER=ecb
SETUP_HTTP_PORT=8000
SETUP_SITE_DOMAIN=http://172.28.0.40:8000
SETUP_TELEPHONY_BACKEND=ami
SETUP_AMI_HOST=172.28.0.20
SETUP_AMI_PORT=5038
SETUP_AMI_USERNAME=ecb
SETUP_AMI_SECRET=labami
SETUP_SSH_PUSH=false
SETUP_ESKIZ_EMAIL=-
SETUP_ADMIN_USER=admin
SETUP_CONFIRM=yes
EOF
}

prep_app() { # prep_app <exec-prefix>
  local X="$1"
  # ishlab turgan servisni to'xtatib binarni yangilaymiz (text file busy'dan qochish)
  $X bash -c 'systemctl stop emergency-callback-web emergency-callback-worker 2>/dev/null; mkdir -p /opt/ecb && cp /artifacts/emergency-callback /opt/ecb/ && chmod +x /opt/ecb/emergency-callback'
}

s1() {
  info "s1: toza o'rnatish (systemd, non-interactive)"
  prep_app "$APP"
  $APP bash -c "cd /opt/ecb && env $(setup_env | tr '\n' ' ') ./emergency-callback setup --non-interactive" \
    || fail "setup s1"
  $APP bash -c 'cd /opt/ecb && ./emergency-callback doctor' && pass "s1 doctor" \
    || { $APP bash -c 'journalctl -u emergency-callback-web -n 20 --no-pager' || true; fail "s1 doctor"; }
}

s2() {
  info "s2: re-run idempotentlik"
  $APP bash -c 'cp /opt/ecb/.env /tmp/env.before; cp /opt/ecb/INSTALL_CREDENTIALS.txt /tmp/creds.before 2>/dev/null || touch /tmp/creds.before'
  $APP bash -c "cd /opt/ecb && env $(setup_env | tr '\n' ' ') ./emergency-callback setup --non-interactive" \
    || fail "setup s2"
  $APP bash -c 'diff /tmp/env.before /opt/ecb/.env' && pass "s2 .env o'zgarmadi" || fail "s2 .env o'zgardi!"
  # yangi credential yaratilmagan re-run creds faylga TEGMASLIGI kerak
  $APP bash -c 'diff /tmp/creds.before /opt/ecb/INSTALL_CREDENTIALS.txt 2>/dev/null || [ ! -f /opt/ecb/INSTALL_CREDENTIALS.txt ]' \
    && pass "s2 INSTALL_CREDENTIALS.txt o'zgarmadi" || fail "s2 credentials fayl o'zgardi!"
}

s3() {
  info "s3: band port"
  $APP bash -c 'nohup python3 -m http.server 8009 >/dev/null 2>&1 & sleep 1' || true
  prep_app "$APP"
  if $APP bash -c "cd /opt/ecb && env $(setup_env | tr '\n' ' ') SETUP_HTTP_PORT=8009 SETUP_PORT_ACTION=next ./emergency-callback setup --non-interactive"; then
    $APP bash -c 'grep -q "HTTP_ADDR=:8010" /opt/ecb/.env' && pass "s3 keyingi portga o'tdi (8010)" || fail "s3 port"
  else
    fail "s3 setup"
  fi
  $APP bash -c 'pkill -f http.server' || true
}

s4() {
  info "s4: systemd yo'q (app-plain)"
  prep_app "$PLAIN"
  $PLAIN bash -c "apt-get update -qq && apt-get install -y -qq postgresql sudo curl >/dev/null" || true
  $PLAIN bash -c "service postgresql start" || true
  $PLAIN bash -c "cd /opt/ecb && env $(setup_env | tr '\n' ' ') SETUP_PG_MODE=peer ./emergency-callback setup --non-interactive" \
    || fail "s4 setup (graceful bo'lishi kerak edi)"
  $PLAIN bash -c 'test -x /opt/ecb/run-web.sh' && pass "s4 run-web.sh yaratilgan" || fail "s4 run script yo'q"
  $PLAIN bash -c 'cd /opt/ecb && nohup ./run-web.sh >/tmp/web.log 2>&1 & sleep 3; curl -sf http://127.0.0.1:8000/users/login/ >/dev/null' \
    && pass "s4 web qo'lda ishga tushdi" || fail "s4 web ishlamadi"
  $PLAIN bash -c 'pkill -f "emergency-callback web"' || true
}

s6() {
  info "s6: ARI probe host'dan (127.0.0.1:8088)"
  curl -sf -u ecb:labari http://127.0.0.1:8088/ari/asterisk/info >/dev/null \
    && pass "s6 ARI auth OK" || fail "s6 ARI javob bermadi"
  info "s6b: res_http_media_cache holati"
  curl -sf -u ecb:labari "http://127.0.0.1:8088/ari/asterisk/modules/res_http_media_cache" >/dev/null \
    && echo " → res_http_media_cache: LOADED" || echo " → res_http_media_cache: YO'Q"
  info "s6c: sounds indeksi"
  curl -sf -u ecb:labari "http://127.0.0.1:8088/ari/sounds/ambulance-rating-request" >/dev/null \
    && echo " → ambulance-rating-request: BOR" || echo " → ambulance-rating-request: YO'Q"
}

e2e() { # e2e <backend> <dtmf_seq> <expected_status>
  local backend="$1" seq="$2" want="$3"
  info "e2e ($backend): DTMF='$seq' → kutilgan status=$want"
  $CALLEE asterisk -rx "dialplan set global DTMF_SEQ \"$seq\"" >/dev/null
  $CALLEE asterisk -rx "dialplan set global OP_MODE answer" >/dev/null

  # Worker va web app konteynerida ishlayapti (s1 dan keyin).
  $APP bash -c "cd /opt/ecb && sed -i 's/^TELEPHONY_BACKEND=.*/TELEPHONY_BACKEND=$backend/' .env"
  if ! $APP bash -c 'grep -q "^ARI_URL=" /opt/ecb/.env'; then
    $APP bash -c 'printf "ARI_URL=http://172.28.0.20:8088/ari\nARI_USERNAME=ecb\nARI_PASSWORD=labari\n" >> /opt/ecb/.env'
  fi
  $APP bash -c 'systemctl restart emergency-callback-web emergency-callback-worker && sleep 2'

  # Demo region/brigada (RandomActiveTeam uchun) — idempotent
  $APP bash -c 'cd /opt/ecb && ./emergency-callback seed >/dev/null 2>&1' || true

  # API orqali callback yaratish
  local resp id
  resp=$($APP bash -c 'curl -sf -X POST http://127.0.0.1:8000/api/create/ -H "Content-Type: application/json" -d "{\"phone_number\":\"+998901234567\"}"')
  echo "  api: $resp"
  id=$(echo "$resp" | grep -o '"callback_id":[0-9]*' | cut -d: -f2)
  [ -n "$id" ] || fail "e2e callback yaratilmadi"

  # Qo'ng'iroq tugashini kutish. Diqqat: SaveRating baho kelgan zahoti
  # statusni 'completed' qiladi (oraliq qiymat) — yakuniy natija esa call
  # tugagach UpdateCallbackResult bilan yoziladi. Shuning uchun terminal
  # status ko'ringach ham qo'shimcha kutamiz va qayta o'qiymiz.
  local status=""
  for i in $(seq 1 40); do
    status=$($APP runuser -u postgres -- psql -d emergency_callback -tAc "SELECT status FROM callbacks_callbackrequest WHERE id=$id" | tr -d '[:space:]')
    case "$status" in pending|dialing|"") sleep 3 ;; *) break ;; esac
  done
  sleep 15
  status=$($APP runuser -u postgres -- psql -d emergency_callback -tAc "SELECT status FROM callbacks_callbackrequest WHERE id=$id" | tr -d '[:space:]')
  echo "  yakuniy status: $status"
  local rating
  rating=$($APP runuser -u postgres -- psql -d emergency_callback -tAc "SELECT rating FROM callbacks_rating WHERE callback_request_id=$id" | tr -d '[:space:]')
  echo "  rating: ${rating:-yoq}"
  [ "$status" = "$want" ] && pass "e2e $backend: $status" || fail "e2e $backend: status=$status, kutilgan=$want"
}

case "${1-}" in
  build) build ;;
  up) up ;;
  provision) provision ;;
  s1) s1 ;;
  s2) s2 ;;
  s3) s3 ;;
  s4) s4 ;;
  s6) s6 ;;
  e2e-ami) e2e ami "ww4" completed ;;
  e2e-ami-transfer) e2e ami "ww4ww0" transferred ;;
  e2e-ari) e2e ari "ww4" completed ;;
  e2e-ari-transfer) e2e ari "ww4ww0" transferred ;;
  # bahosiz: no_rating → SMS job → status waiting_rating (SMS havola kutilyapti)
  e2e-ari-norating) e2e ari "" waiting_rating ;;
  down) $C down ;;
  nuke) $C down -v ;;
  *) grep '^#' "$0" | head -30; exit 2 ;;
esac
