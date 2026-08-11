#!/usr/bin/env bash
#
# FreePBX konteyneri ICHIDA ishlaydi (docker exec):
#   - test extension'lar 200/337 (raw pjsip custom conf — GUI'siz, deterministik)
#   - from-internal-custom: 901234567 → PJSIP/200, 337/777 → PJSIP/337
#   - AMI user (ecb/labami) + 3 ta app konteksti (AMI backend uchun)
#   - ARI user (ecb/labari) + HTTP server 8088
#   - audio prompts → sounds katalogi
#
# Idempotent: marker satrlar bo'yicha qayta yozmaydi.
set -euo pipefail

A=/etc/asterisk
MARK="; ecb-lab"

append_once() { # append_once <file> <content>
  local f="$1" content="$2"
  grep -qF "$MARK" "$f" 2>/dev/null || printf '%s\n%s\n' "$MARK" "$content" >> "$f"
}

echo "==> pjsip custom endpoints (200, 337)"
# FreePBX pjsip.conf aynan *_custom_post.conf fayllarni include qiladi
# (pjsip.endpoint_custom.conf EMAS — u umuman o'qilmaydi!)
append_once "$A/pjsip.endpoint_custom_post.conf" "
[200]
type=endpoint
context=from-internal
disallow=all
allow=ulaw,alaw
auth=auth200
aors=200
dtmf_mode=rfc4733

[337]
type=endpoint
context=from-internal
disallow=all
allow=ulaw,alaw
auth=auth337
aors=337
dtmf_mode=rfc4733
"
append_once "$A/pjsip.auth_custom_post.conf" "
[auth200]
type=auth
auth_type=userpass
username=200
password=labpass200

[auth337]
type=auth
auth_type=userpass
username=337
password=labpass337
"
append_once "$A/pjsip.aor_custom_post.conf" "
[200]
type=aor
max_contacts=2
remove_existing=yes

[337]
type=aor
max_contacts=2
remove_existing=yes
"

echo "==> transport local_net (docker subnet — aks holda FreePBX external_media_address ishlatib RTP'ni tashqariga yo'naltiradi)"
append_once "$A/pjsip.transports_custom_post.conf" "
[0.0.0.0-udp](+)
local_net = 172.28.0.0/16
"

echo "==> dialplan (from-internal-custom + app kontekstlari)"
append_once "$A/extensions_custom.conf" "
[from-internal-custom]
; test raqami → callee (200)
exten => 901234567,1,NoOp(LAB dial to callee)
 same => n,Dial(PJSIP/200,25)
 same => n,Hangup()
exten => 337,1,Dial(PJSIP/337,25)
exten => 777,1,Dial(PJSIP/337,25)

[ambulance-callback]
exten => s,1,NoOp(ANSWERED CALL_ID=\${CALL_ID} PHONE=\${PHONE_NUMBER})
 same => n,Answer()
 same => n,UserEvent(CallAnswered,CallID: \${CALL_ID},Phone: \${PHONE_NUMBER})
 same => n,Wait(300)
 same => n,Hangup()

[play-audio]
exten => _.,1,NoOp(PLAY \${EXTEN} CALL_ID=\${CALL_ID})
 same => n,Playback(\${EXTEN})
 same => n,UserEvent(AudioPlayed,CallID: \${CALL_ID},Audio: \${EXTEN})
 same => n,WaitExten(60)
 same => n,Wait(60)
 same => n,Hangup()

[transfer-to-337]
exten => s,1,NoOp(TRANSFER CALL_ID=\${CALL_ID})
 same => n,Dial(Local/337@from-internal,30)
 same => n,Hangup()
"

echo "==> AMI user"
append_once "$A/manager_custom.conf" "
[ecb]
secret = labami
deny = 0.0.0.0/0.0.0.0
permit = 0.0.0.0/0.0.0.0
read = system,call,log,verbose,agent,user,config,dtmf,reporting,cdr,dialplan,originate
write = system,call,agent,user,config,command,reporting,originate,message
"

echo "==> HTTP server (8088) + ARI user"
# FreePBX'ning rasmiy yo'li — Advanced Settings (mijoz FreePBX'ida GUI'da
# xuddi shu ikkita sozlama yoqiladi: 'Enable Asterisk mini-HTTP' + bind).
if command -v fwconsole >/dev/null 2>&1; then
  fwconsole setting HTTPENABLED 1 >/dev/null
  fwconsole setting HTTPBINDADDRESS 0.0.0.0 >/dev/null
else
  append_once "$A/http_custom.conf" "
[general]
enabled=yes
bindaddr=0.0.0.0
bindport=8088
"
fi
if grep -q "ari_additional_custom.conf" "$A/ari.conf" 2>/dev/null || [ -f "$A/ari_general_additional.conf" ]; then
  # FreePBX uslubi: ari.conf → ari_general_additional + ari_additional_custom
  append_once "$A/ari_additional_custom.conf" "
[ecb]
type = user
read_only = no
password = labari
"
else
  append_once "$A/ari.conf" "
[general]
enabled = yes

[ecb]
type = user
read_only = no
password = labari
"
fi

echo "==> audio prompts"
SOUNDS=/var/lib/asterisk/sounds/en
mkdir -p "$SOUNDS"
cp -f /lab-audios/ambulance-*.wav "$SOUNDS/" 2>/dev/null || echo " ! /lab-audios topilmadi"
chown asterisk:asterisk "$SOUNDS"/ambulance-*.wav 2>/dev/null || true

echo "==> reload"
fwconsole reload 2>/dev/null || asterisk -rx 'core reload' || true
asterisk -rx 'manager reload' >/dev/null 2>&1 || true
asterisk -rx 'module reload res_http_websocket' >/dev/null 2>&1 || true
asterisk -rx 'module reload res_ari' >/dev/null 2>&1 || true

echo "==> tekshiruv"
asterisk -rx 'pjsip show endpoints' | grep -E "200|337" | head -4 || true
asterisk -rx 'manager show user ecb' | head -3 || true
asterisk -rx 'ari show users' 2>/dev/null | head -5 || true
asterisk -rx 'http show status' | head -6 || true
echo "provision OK"
