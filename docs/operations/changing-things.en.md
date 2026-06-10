# Changing Things (Cookbook)

Task-oriented recipes for the common "I want to change X — what happens?"
questions. Each entry says **where** the change lives, **whether it needs a
rebuild**, and **how to apply** it.

Legend:

- 🟢 **Config/asset only** — no code change, no rebuild.
- 🟡 **Asterisk change** — edit dialplan/PBX, reload Asterisk.
- 🔴 **Code change** — edit Go source, rebuild the binary, restart.

---

## Change an audio prompt 🟢

Re-record one of the six prompts (same base name, WAV 8 kHz mono 16-bit), copy
it over the installed file, fix ownership. No reload — `Playback` reads the file
on each call.

```bash
sudo cp ambulance-rating-request.wav /var/lib/asterisk/sounds/en/
sudo chown asterisk:asterisk /var/lib/asterisk/sounds/en/ambulance-rating-request.wav
cp ambulance-rating-request.wav /opt/emergency_callback/audios/   # keep repo copy in sync
```

Full details + format conversion: [Audio Prompts](../telephony/audio-prompts.md).

---

## Change the SMS text 🔴

The SMS body is a literal string in `internal/jobs/sms.go` (the Karakalpak
message with the `%s` vote-URL placeholder). Edit it, then rebuild and restart
the worker.

```go
// internal/jobs/sms.go
body := fmt.Sprintf(
    "Assalawma aleykum. ... %s",   // <-- edit this text; keep the %s for the URL
    voteURL,
)
```

```bash
go build -o emergency-callback ./cmd/emergency-callback
sudo systemctl restart emergency-callback-worker
```

!!! warning
    Keep exactly one `%s` — it is where the vote link is inserted.

---

## Change a Russian UI message 🔴

Admin-facing messages (flash notices, errors) are inline string literals in the
handlers, e.g. `internal/handlers/callbacks.go`:

```go
s.pushFlash(c, "success", "Экстренный вызов создан! Звоним на номер "+phone+"...")
```

Edit the string(s), rebuild, restart the web service. Page labels live in the
`templates/` HTML files — edit those and restart `web` (templates load at
startup).

```bash
go build -o emergency-callback ./cmd/emergency-callback
sudo systemctl restart emergency-callback-web
```

---

## Change rating retries / call timeout 🟢

These are environment variables — edit `.env`, restart the worker.

```bash
AMI_RATING_RETRY_LIMIT=3     # invalid keypresses tolerated before giving up
AMI_RATING_TIMEOUT=10        # seconds to wait for rating input
AMI_CALL_TIMEOUT=60          # seconds before a call is abandoned
```

```bash
sudo systemctl restart emergency-callback-worker
```

The retry limit is enforced in the AMI bridge; the timeouts bound how long the
worker waits. See [Configuration](../getting-started/configuration.md).

---

## Change the operator transfer target 🟡

The transfer destination is in the `transfer-to-337` dialplan context, not in
the app. Edit it and reload Asterisk.

```ini
[transfer-to-337]
exten => s,1,NoOp(TRANSFER CALL_ID=${CALL_ID})
 same => n,Dial(Local/777@from-internal,30)   ; <-- your operator queue/extension
 same => n,Hangup()
```

```bash
sudo fwconsole reload     # or: sudo asterisk -rx 'dialplan reload'
```

!!! note
    Keep the **context name** `transfer-to-337` — the app redirects to it by
    name. Only change the `Dial(...)` target.

---

## Change the country-code prefix or dial format 🔴🟡

Two places cooperate:

1. **Strip on the app side** — `internal/ami/bridge.go`, `formatPhoneNumber()`
   drops a leading `998` from 12-digit numbers before originating:
   ```go
   if len(s) == 12 && s[:3] == "998" {
       return s[3:]
   }
   ```
   For a different country code, change `"998"` (and the length check), rebuild,
   restart the worker.

2. **Prepend on the dialplan/route side** — on FreePBX, your Outbound Route's
   dial pattern prepends what the trunk expects; on standalone Asterisk, the
   `from-internal` context does `Dial(PJSIP/998${EXTEN}@trunk-endpoint,...)`.
   Adjust the prefix there and reload Asterisk.

Verify with the worker log line `ami originated phone=<digits>` — that is exactly
what arrives at `from-internal`.

---

## Point at a different Asterisk / AMI user 🟢

Edit the `AMI_*` values in `.env`, restart the worker. Make sure the AMI user
exists on that Asterisk with the right permissions (especially `dtmf` read).

```bash
AMI_HOST=10.0.0.5
AMI_PORT=5038
AMI_USERNAME=ecb
AMI_SECRET=...
```

See [FreePBX Integration](../telephony/freepbx-integration.md).

---

## Change the trunk 🟡

Trunk configuration lives entirely in Asterisk/FreePBX (`pjsip.conf` or the
FreePBX Trunks UI) and outbound routing. The app does not know about trunks — it
only originates into `from-internal`. Update the trunk/route in the PBX and
reload. Mind the [PJSIP gotchas](../telephony/standalone-asterisk.md) if editing
`pjsip.conf` by hand.

---

## Change the web port or public URL 🟢

```bash
HTTP_ADDR=127.0.0.1:8000              # listen address
SITE_DOMAIN=https://callback.example.com   # used in SMS vote links
```

Restart `web`. If behind a proxy, update the proxy too. `SITE_DOMAIN` must match
the externally reachable URL or SMS links break.

---

## Add a team or region 🟢

Use the admin UI: **Регионы** / **Бригады** (`/teams/regions/`, `/teams/`). No
restart. New active teams become eligible for new callbacks immediately. See the
[Admin Guide](../usage/admin-guide.md).

---

## Add or change a user / role 🟢

From the server CLI:

```bash
./emergency-callback createuser <username> <password> [admin|operator]
```

There is no in-app user editor; role is set at creation. To change a role,
update the `role` column directly in the database (`admin` or `operator`) or
recreate the user.

---

## Change a database field / add a feature 🔴

Schema changes mean a new goose migration under `migrations/`, regenerated typed
queries (`sqlc`), and Go changes. This is development work beyond configuration —
add a migration, run `migrate up`, update the queries and handlers, rebuild.
(Schema reference: [Database Schema](../reference/database-schema.md).)

---

## Quick reference: what requires what

| Change | Type | Apply with |
|--------|------|-----------|
| Audio prompt | 🟢 | copy file (no reload) |
| SMS text | 🔴 | rebuild + restart worker |
| Russian message / labels | 🔴 | rebuild + restart web |
| Retry / timeouts | 🟢 | `.env` + restart worker |
| Transfer target | 🟡 | dialplan reload |
| Country code | 🔴🟡 | rebuild + dialplan/route |
| AMI target | 🟢 | `.env` + restart worker |
| Trunk | 🟡 | PBX + reload |
| Web port / URL | 🟢 | `.env` + restart web |
| Teams / regions | 🟢 | admin UI |
| Users / roles | 🟢 | CLI / DB |
| Schema / feature | 🔴 | migration + rebuild |
