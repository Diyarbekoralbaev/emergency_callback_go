# ARI backend (no dialplan)

The app supports two call-control backends (`TELEPHONY_BACKEND`):

| | `ami` (classic) | `ari` (recommended for new installs) |
|---|---|---|
| PBX-side setup | AMI user + **3 dialplan contexts** + WAV files | **an ARI user only** |
| Audio | WAV files on the PBX | over HTTP from the app server*, or WAVs on the PBX |
| DTMF | events from every bridge leg + dedupe | one event per keypress |
| Operator transfer | `transfer-to-337` dialplan context | app-created bridge; target = `AMI_OPERATOR_QUEUE` |

\* requires the `res_http_media_cache` module (loaded by default on
FreePBX 17 / Asterisk 21; check with
`asterisk -rx 'module show like res_http_media_cache'`).

## How it works

1. The app opens the `ARI_URL/events` WebSocket with a **unique Stasis app
   name per call** — events are isolated.
2. `POST /channels` with `endpoint=Local/<number>@from-internal/n` and
   `app=<name>` — FreePBX Outbound Routes are reused as-is, **no custom
   contexts needed**.
3. Callee answers → `StasisStart` → rating prompt.
4. `ChannelDtmfReceived`: `1–5` saves the rating; `0`/`9` after the thank-you
   triggers transfer: the app creates a mixing bridge and dials the operator
   (`Local/<AMI_OPERATOR_QUEUE>@from-internal`).
5. Silence after a prompt (`AMI_RATING_TIMEOUT` s) → re-prompt up to
   `AMI_RATING_RETRY_LIMIT`, then finish as `no_rating` (SMS fallback).

## The audio ladder (resolved per call)

1. **HTTP**: `AUDIO_MEDIA_BASE_URL` set **and** `res_http_media_cache`
   loaded → Asterisk fetches `<base>/call-media/<name>.wav` from the app.
   No files on the PBX; admin-page uploads take effect immediately.
2. **sound:** files exist in the PBX sounds dir (copied by `setup` over SSH
   or manually).
3. **custom/**: files uploaded via FreePBX GUI → System Recordings.

If no rung works, the call fails with a precise worker-log error naming the
fix.

## FreePBX setup (GUI only)

1. **Settings → Asterisk REST Interface Users → Add User** — the
   `ARI_USERNAME`/`ARI_PASSWORD` pair, Read Only = No.
2. **Settings → Advanced Settings**: enable *the Asterisk Builtin mini-HTTP
   server* and set the bind address to `0.0.0.0` (by default ARI listens on
   `127.0.0.1:8088` only).
3. Open **8088/tcp** from the app server to the PBX.
4. Verify from the app server:
   ```bash
   curl -u <user>:<pass> http://<freepbx-ip>:8088/ari/asterisk/info
   ./emergency-callback doctor
   ```

!!! warning "One-way audio / no DTMF"
    If the PBX sits behind NAT with an external address configured in SIP
    Settings and the app server's network is not listed in Local Networks,
    RTP goes to the external address and DTMF never arrives. Add the subnet
    under **Settings → Asterisk SIP Settings → Local Networks**.

## .env

```ini
TELEPHONY_BACKEND=ari
ARI_URL=http://172.16.95.250:8088/ari
ARI_USERNAME=ecb
ARI_PASSWORD=...
# optional, enables HTTP audio (rung 1):
AUDIO_MEDIA_BASE_URL=http://<app-server>:8000
```

AMI variables are not required with `ari`; `AMI_CALL_TIMEOUT`,
`AMI_RATING_RETRY_LIMIT`, `AMI_RATING_TIMEOUT`, `AMI_OPERATOR_QUEUE` and
`AMI_CALLER_ID` are shared by both backends.
