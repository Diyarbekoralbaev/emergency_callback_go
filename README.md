# Emergency Callback (Go)

Go rewrite of the Django + Celery emergency-callback system. Single binary,
PostgreSQL-only, no Redis.

## Stack

| Layer | Library |
|-------|---------|
| Web | gin-gonic/gin |
| DB | pgx/v5 + sqlc + goose |
| Job queue | riverqueue/river (Postgres-backed) |
| Asterisk AMI | staskobzar/goami2 |
| Telephony | AMI (goami2) or ARI (Stasis, no dialplan) |
| Sessions | alexedwards/scs/v2 + pgxstore |
| Templates | html/template (stdlib) |
| CSRF | gorilla/csrf |
| Excel | xuri/excelize/v2 |
| SMS | DIY HTTP to Eskiz.uz |

## Layout

```
cmd/emergency-callback/  entrypoint with web/worker/createuser/seed/migrate subcommands
internal/
  ami/         goami2 client wrapping the Asterisk call state machine
  ari/         ARI/Stasis backend (no dialplan on the PBX)
  auth/        bcrypt + scs + middleware
  config/      .env loader
  db/          pgxpool + sqlc queries
  handlers/    HTTP handlers
  jobs/        River workers (ProcessCallback, SendRatingSMS, CleanupStaleCalls)
  models/      template-friendly view structs
  pbxssh/      audio push to the PBX over SSH
  sms/         Eskiz HTTP client
  server/      Gin router + middleware wiring
  setup/       install wizard (setup) + health checks (doctor)
  telephony/   backend-neutral Caller contract (AMI/ARI switch)
  templates/   html/template loader + funcs
  tz/          Asia/Tashkent helpers
migrations/    goose .sql files (schema matches Django 1:1)
templates/     19 HTML files (Bootstrap 5 from CDN)
lab/           docker test lab: FreePBX 17 + callee + scenario matrix
```

## Setup

> **Note:** `sudo ./install.sh` does all of the below automatically via the
> built-in `setup` wizard (downloads a Release binary or builds locally).

1. Postgres: create the database
   ```bash
   createdb emergency_callback_go
   ```
2. Configure `.env` (copy from `.env.example` and fill DB password, Eskiz creds, etc.)
3. Build:
   ```bash
   go build -o emergency-callback ./cmd/emergency-callback
   ```
4. Run migrations (River job-queue tables are migrated in-process too — no
   `river` CLI needed):
   ```bash
   ./emergency-callback migrate up
   ```
5. Create the first admin user:
   ```bash
   ./emergency-callback createuser admin admin123 admin
   ```
6. (Optional) Seed demo data:
   ```bash
   ./emergency-callback seed
   ```

## Running

Two processes — web and worker — typically managed by systemd:

```bash
# Terminal 1 — HTTP server
./emergency-callback web

# Terminal 2 — River job worker
./emergency-callback worker
```

The web process serves the dashboard at `http://localhost:8000`. The worker
process runs the three jobs:
- `process_callback_call` — drives the AMI call
- `send_rating_sms` — fallback SMS via Eskiz
- `cleanup_stale_calls` — periodic (every 15min) cleanup of stuck calls

## Deploying as a single binary

Strip and trim for the smallest binary:

```bash
CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath -o emergency-callback ./cmd/emergency-callback
```

Result: ~30MB binary. Copy alongside `templates/`, `migrations/`, and `.env`
to the production server. systemd unit example:

```ini
[Unit]
Description=Emergency Callback (web)
After=network.target postgresql.service

[Service]
Type=simple
User=callback
WorkingDirectory=/opt/emergency_callback
ExecStart=/opt/emergency_callback/emergency-callback web
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

(Duplicate for the worker.)

## Environment variables

See `.env.example` for the full list. Notable:

| Var | Purpose |
|-----|---------|
| `DATABASE_URL` | Postgres connection string |
| `HTTP_ADDR` | Listen address (default `:8000`) |
| `SITE_DOMAIN` | Used in the SMS vote URLs |
| `SESSION_SECRET` | scs encryption key (32+ bytes) |
| `CSRF_KEY` | gorilla/csrf key (32 bytes) |
| `TELEPHONY_BACKEND` | `ami` (dialplan on the PBX) or `ari` (Stasis, no dialplan) |
| `AMI_*` | Asterisk Manager Interface credentials + dialplan |
| `ARI_*` | Asterisk REST Interface URL + credentials (ARI backend) |
| `AUDIO_MEDIA_BASE_URL` | Serve voice prompts to the PBX over HTTP (ARI backend) |
| `COOKIE_SECURE` | Set `true` behind an HTTPS proxy (TLS-only session cookie) |
| `ESKIZ_*` | SMS provider credentials |
| `RIVER_MAX_WORKERS` | River concurrency per queue |

## What's preserved from the Django app

- All URL paths (`/callbacks/`, `/teams/`, `/vote/<uuid>/`, etc.)
- Database schema (field names, types, indexes) — identical 1:1
- AMI dialplan: `ambulance-callback`, `play-audio`, `transfer-to-337`
- Audio extension names (`ambulance-rating-request` etc.)
- Russian admin strings; Karakalpak user-facing strings
- Asia/Tashkent timezone handling on UI; UTC in DB

## What's dropped

- Django admin (`/admin/`) and the 5 "Админ" escape-hatch buttons in templates
- Celery + Redis (replaced by River + Postgres)
- `tests.py` (Django default empty stubs)
- `listen.py` (RabbitMQ systemctl service controller — separate concern)
