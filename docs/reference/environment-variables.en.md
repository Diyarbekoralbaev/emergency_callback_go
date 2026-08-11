# Environment Variables

All configuration comes from a `.env` file (or real environment variables) read
at startup. Explanations and effects are in
[Configuration](../getting-started/configuration.md); this is the quick table.

!!! warning
    Values are plain `KEY=value`. Unquoted `<`, `>`, or spaces break the dotenv
    parser and cause `required env var ... not set` on startup.

## Database

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | yes | — | PostgreSQL DSN, e.g. `postgres://ecb:pass@127.0.0.1:5432/emergency_callback?sslmode=disable`. |
| `DB_POOL_MAX_CONNS` | no | `100` | Max pooled connections. |
| `DB_POOL_MIN_CONNS` | no | `10` | Min pooled connections. |

## HTTP server

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `HTTP_ADDR` | no | `:8000` | Listen address. Use `127.0.0.1:8000` behind a proxy. |
| `SITE_DOMAIN` | yes | — | Public base URL; used to build SMS vote links. No trailing slash. |
| `SESSION_SECRET` | yes | — | Session cookie encryption key (32+ bytes). Changing it logs everyone out. |
| `CSRF_KEY` | yes | — | CSRF key; must be **exactly 32 bytes** (`openssl rand -base64 24`). |
| `COOKIE_SECURE` | no | `false` | Set `true` behind an HTTPS proxy: session cookie is TLS-only. |

## Telephony

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `TELEPHONY_BACKEND` | no | `ami` | `ami` (classic, dialplan on the PBX) or `ari` (Stasis, no dialplan needed). |

## Asterisk AMI (required when `TELEPHONY_BACKEND=ami`)

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `AMI_HOST` | for ami | — | AMI host (often `127.0.0.1`). |
| `AMI_PORT` | no | `5038` | AMI port. |
| `AMI_USERNAME` | for ami | — | AMI user (must exist in Asterisk). |
| `AMI_SECRET` | for ami | — | AMI secret (must match Asterisk). |
| `AMI_CALLER_ID` | no | — | Caller ID number presented outbound. Bare number/string only. |
| `AMI_OPERATOR_QUEUE` | no | `777` | Operator destination for the ARI backend (extension/queue in `from-internal`). In AMI mode the target lives in the dialplan. |
| `AMI_CALL_TIMEOUT` | no | `60` | Seconds before a call is abandoned (shared by both backends). |
| `AMI_RATING_RETRY_LIMIT` | no | `3` | Invalid keypresses tolerated before giving up (shared). |
| `AMI_RATING_TIMEOUT` | no | `10` | ARI: seconds of silence after a prompt before re-prompt/finish. |

## Asterisk ARI (required when `TELEPHONY_BACKEND=ari`)

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `ARI_URL` | for ari | — | E.g. `http://<freepbx-ip>:8088/ari`. |
| `ARI_USERNAME` | for ari | — | ARI user (FreePBX: Settings → Asterisk REST Interface Users). |
| `ARI_PASSWORD` | for ari | — | ARI password. |

## Audio

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `AUDIO_DIR` | no | `audios` | WAV prompts directory on the app server. |
| `AUDIO_MEDIA_BASE_URL` | no | — | If set and `res_http_media_cache` is loaded on the PBX, the ARI backend plays prompts over HTTP from this base URL (usually = `SITE_DOMAIN`); no files needed on the PBX. |
| `PBX_SSH_HOST` | no | — | PBX SSH host for auto-push of admin-uploaded audio (`sound:` mode). |
| `PBX_SSH_USER` | no | — | SSH user. |
| `PBX_SSH_PASSWORD` / `PBX_SSH_KEY` | no | — | Password or path to a private key. |
| `PBX_SOUNDS_DIR` | no | `/var/lib/asterisk/sounds/en` | Asterisk sounds directory on the PBX. |

## Eskiz SMS

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `ESKIZ_EMAIL` | for SMS | — | Eskiz account email. |
| `ESKIZ_PASSWORD` | for SMS | — | Eskiz account password. |
| `ESKIZ_BASE_URL` | no | `https://notify.eskiz.uz/api` | API base URL. |
| `ESKIZ_DRY_RUN` | no | `false` | `true` logs SMS instead of sending. |

## Workers

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `RIVER_MAX_WORKERS` | no | `5` | Max concurrent background jobs (in-flight calls). |

## Build-time / special

| Variable | Where | Description |
|----------|-------|-------------|
| `MIGRATIONS_DIR` | `migrate` subcommand | Override the migrations directory (default `migrations`). |
| `ENABLE_PDF_EXPORT` | docs build | Set to `1` to also render the docs site to PDF (`mkdocs build`). |
