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

## Asterisk AMI

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `AMI_HOST` | yes | — | AMI host (often `127.0.0.1`). |
| `AMI_PORT` | no | `5038` | AMI port. |
| `AMI_USERNAME` | yes | — | AMI user (must exist in Asterisk). |
| `AMI_SECRET` | yes | — | AMI secret (must match Asterisk). |
| `AMI_CALLER_ID` | no | — | Caller ID number presented outbound. Bare number/string only. |
| `AMI_OPERATOR_QUEUE` | no | `777` | Operator destination hint. |
| `AMI_CALL_TIMEOUT` | no | `60` | Seconds before a call is abandoned. |
| `AMI_RATING_RETRY_LIMIT` | no | `3` | Invalid keypresses tolerated before giving up. |
| `AMI_RATING_TIMEOUT` | no | `10` | Seconds to wait for rating input. |

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
