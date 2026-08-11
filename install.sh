#!/usr/bin/env bash
#
# Emergency Callback — bootstrap.
#
# Bu skript faqat binarni topadi/yuklaydi va haqiqiy o'rnatuvchini ishga
# tushiradi. Butun o'rnatish mantig'i Go'dagi interaktiv wizard'da:
#   ./emergency-callback setup
#
#   sudo ./install.sh              # binar tayyorlab, setup'ni ishga tushiradi
#   sudo ./install.sh --download   # majburan Releases'dan yuklash
#   sudo ./install.sh --build      # majburan lokal build (Go kerak)
#
set -euo pipefail

REPO="Diyarbekoralbaev/emergency_callback_go"
BIN="emergency-callback"
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$DIR"

err() { echo " ✗ $*" >&2; }
info() { echo "==> $*"; }

[ "$(id -u)" = 0 ] || { err "sudo bilan ishga tushiring: sudo ./install.sh"; exit 1; }
command -v curl >/dev/null || { err "curl kerak (apt-get install -y curl)"; exit 1; }

MODE="auto"
case "${1-}" in
  --download) MODE=download ;;
  --build)    MODE=build ;;
  -h|--help)  awk 'NR>1 && /^#/{sub(/^# ?/,"");print} NR>1 && !/^#/{exit}' "$0"; exit 0 ;;
  "") ;;
  *) err "noma'lum flag: $1"; exit 2 ;;
esac

# go.mod'dagi minimal Go versiyasi (build rejimi uchun)
required_go() { awk '/^go /{print $2}' go.mod 2>/dev/null; }

go_ok() {
  command -v go >/dev/null 2>&1 || return 1
  # GOTOOLCHAIN=auto to'g'ri versiyani o'zi yuklaydi — mavjudligi yetarli
  return 0
}

download_release() {
  info "GitHub Releases'dan yuklanmoqda…"
  local url="https://github.com/${REPO}/releases/latest/download"
  curl -fsSL "${url}/emergency-callback_linux_amd64" -o "$BIN.tmp"
  curl -fsSL "${url}/checksums.txt" -o checksums.txt.tmp
  local want got
  want="$(awk '{print $1}' checksums.txt.tmp)"
  got="$(sha256sum "$BIN.tmp" | awk '{print $1}')"
  rm -f checksums.txt.tmp
  if [ "$want" != "$got" ]; then
    rm -f "$BIN.tmp"
    err "checksum mos kelmadi (yuklash buzilgan?)"; exit 1
  fi
  mv "$BIN.tmp" "$BIN" && chmod +x "$BIN"
  info "Binar tayyor: ./$BIN ($(./$BIN version))"
}

build_local() {
  info "Lokal build (go.mod: go $(required_go))…"
  go build -o "$BIN" ./cmd/emergency-callback
  info "Binar tayyor: ./$BIN"
}

case "$MODE" in
  download) download_release ;;
  build)
    go_ok || { err "Go topilmadi — --download ishlating yoki Go o'rnating"; exit 1; }
    build_local ;;
  auto)
    if [ -x "$BIN" ]; then
      info "Mavjud binar ishlatiladi: ./$BIN ($(./$BIN version 2>/dev/null || echo '?'))"
    elif [ -d cmd/emergency-callback ] && go_ok; then
      build_local
    else
      download_release
    fi ;;
esac

exec "./$BIN" setup "$@"
