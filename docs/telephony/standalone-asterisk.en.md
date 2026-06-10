# Standalone Asterisk (test only)

!!! warning "For local testing only"
    Production is expected to integrate with your existing FreePBX — see
    [FreePBX Integration](freepbx-integration.md). Use this page only to stand up
    a bare Asterisk for development/testing when no FreePBX is available.

The difference from the FreePBX setup: there is **no FreePBX outbound routing**,
so you must define the `from-internal` context yourself to dial your trunk.

## 1. Install Asterisk

```bash
sudo apt install -y asterisk
```

## 2. AMI user — `/etc/asterisk/manager.conf`

```ini
[general]
enabled = yes
port = 5038
bindaddr = 127.0.0.1
displayconnects = no
allowmultiplelogin = yes
authtimeout = 30
authlimit = 50

[ecb]
secret = CHANGE_ME_AMI_SECRET
deny = 0.0.0.0/0.0.0.0
permit = 127.0.0.1/255.255.255.255
read = system,call,log,verbose,agent,user,config,dtmf,reporting,cdr,dialplan,originate
write = system,call,agent,user,config,command,reporting,originate,message
```

## 3. SIP transport + trunk — `/etc/asterisk/pjsip.conf`

```ini
[transport-udp]
type=transport
protocol=udp
bind=0.0.0.0:5060            ; use a free port (e.g. :5062) if 5060 is taken

[trunk-auth]
type=auth
auth_type=userpass
username=<trunk-username>
password=<trunk-password>

[trunk-aor]
type=aor
contact=sip:<provider-host>
qualify_frequency=0          ; provider ignores OPTIONS → keep static contact usable

[trunk-endpoint]
type=endpoint
transport=transport-udp
context=default
disallow=all
allow=ulaw
allow=alaw
outbound_auth=trunk-auth
aors=trunk-aor
from_user=<trunk-username>
from_domain=<provider-host>
direct_media=no
rtp_symmetric=yes
force_rport=yes
rewrite_contact=yes
dtmf_mode=auto

; If your provider requires registration:
[trunk-reg]
type=registration
transport=transport-udp
outbound_auth=trunk-auth
server_uri=sip:<provider-host>
client_uri=sip:<trunk-username>@<provider-host>
retry_interval=60
expiration=3600
```

!!! danger "PJSIP config gotchas (will silently break calls)"
    - **No invalid options.** `rxgain`/`txgain` are *not* valid on a PJSIP
      `type=endpoint`; one invalid line stops the whole endpoint from loading
      (`endpoint '<name>' was not found`).
    - **No stray lines.** Any non-`key=value`, non-`[section]` line breaks the
      parser for everything after it — silently dropping trunks defined below.
    - **`qualify_frequency=0`** if the provider doesn't answer SIP OPTIONS,
      otherwise the contact goes `Unavailable` and dials fail with
      `Could not create dialog to invalid URI '<aor>'`.
    - **Port conflicts.** Only one process can bind UDP `5060`. If it's taken,
      bind the transport to another port (e.g. `0.0.0.0:5062`); outbound still
      works.

## 4. Dialplan — `/etc/asterisk/extensions.conf`

Unlike FreePBX, you **define `from-internal` yourself** to dial the trunk
(prepend the country code as your provider expects):

```ini
[from-internal]
exten => _X.,1,NoOp(OUTBOUND ${EXTEN} via trunk-endpoint  CALL_ID=${CALL_ID})
 same => n,Dial(PJSIP/998${EXTEN}@trunk-endpoint,60)
 same => n,Hangup()

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
 same => n,Dial(PJSIP/<operator-endpoint>,30)
 same => n,Hangup()
```

## 5. Audio + reload

```bash
sudo cp audios/ambulance-*.wav /usr/share/asterisk/sounds/en/
sudo chown asterisk:asterisk /usr/share/asterisk/sounds/en/ambulance-*.wav
sudo systemctl restart asterisk
```

## 6. Verify

```bash
sudo asterisk -rx 'manager show users'        # ecb listed
sudo asterisk -rx 'pjsip show endpoints'      # trunk-endpoint, "Not in use"
sudo asterisk -rx 'pjsip show transports'     # transport-udp bound
sudo asterisk -rx 'dialplan show ambulance-callback'
```

Then test exactly as in [FreePBX Integration → Verify](freepbx-integration.md#step-5-verify-end-to-end).

!!! note "Don't run two Asterisks at once"
    If a containerized PBX or another Asterisk already holds UDP 5060 on the
    host, this standalone instance cannot bind it. Either stop the other one or
    bind this transport to a different port. Two instances fighting over 5060
    surfaces as `Unable to retrieve PJSIP transport 'transport-udp'` /
    `Address already in use` and **all outbound calls fail**.
