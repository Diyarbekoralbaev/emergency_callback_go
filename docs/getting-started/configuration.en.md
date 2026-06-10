# Configuration

All configuration lives in a single `.env` file in the working directory of the
binary. It is loaded at startup. A concise table is in
[Environment Variables](../reference/environment-variables.md); this page
explains each value and **what changes if you change it**.

!!! warning "`.env` syntax"
    Plain `KEY=value`, one per line. No surrounding quotes are needed and
    unquoted special characters (`<`, `>`, spaces) **break parsing**. A broken
    line makes later variables look "missing" and the app panics on startup with
    `required env var ... not set`.

## Database

```bash
DATABASE_URL=postgres://ecb:PASSWORD@127.0.0.1:5432/emergency_callback?sslmode=disable
DB_POOL_MAX_CONNS=100
DB_POOL_MIN_CONNS=10
```

- **`DATABASE_URL`** — full PostgreSQL DSN. Used by the web server, the worker,
  migrations, and River. If wrong you get `password authentication failed` or
  connection errors at startup.
- **`DB_POOL_MAX_CONNS` / `DB_POOL_MIN_CONNS`** — pgx connection-pool bounds.
  Raise the max if you run many concurrent workers; keep it below PostgreSQL's
  `max_connections`.

## HTTP server

```bash
HTTP_ADDR=:8000
SITE_DOMAIN=https://your-public-domain.example
SESSION_SECRET=<32+ random bytes>
CSRF_KEY=<exactly 32 random bytes>
```

- **`HTTP_ADDR`** — listen address. Behind a reverse proxy, bind a localhost
  port (e.g. `127.0.0.1:8000`). If the port is busy the server logs
  `address already in use` and exits.
- **`SITE_DOMAIN`** — the public base URL. **Used to build SMS vote links**
  (`<SITE_DOMAIN>/vote/<uuid>`). If wrong, SMS recipients get an unreachable
  link. No trailing slash.
- **`SESSION_SECRET`** — encryption key for session cookies. **Changing it logs
  everyone out** (existing sessions become invalid).
- **`CSRF_KEY`** — must be **exactly 32 bytes**. Changing it invalidates
  in-flight form submissions (users just reload). Generate with
  `openssl rand -base64 24`.

## Asterisk AMI

```bash
AMI_HOST=127.0.0.1
AMI_PORT=5038
AMI_USERNAME=ecb
AMI_SECRET=<secret>
AMI_CALLER_ID=781138081
AMI_OPERATOR_QUEUE=777
AMI_CALL_TIMEOUT=60
AMI_RATING_RETRY_LIMIT=3
AMI_RATING_TIMEOUT=10
```

- **`AMI_HOST` / `AMI_PORT`** — where Asterisk's AMI listens. Usually
  `127.0.0.1:5038` when the worker runs on the PBX host.
- **`AMI_USERNAME` / `AMI_SECRET`** — must match the AMI user you create in
  Asterisk ([FreePBX Integration](../telephony/freepbx-integration.md)). Wrong
  credentials → the worker cannot originate calls.
- **`AMI_CALLER_ID`** — the caller ID number presented on the outbound call.
  Keep it a bare number/string (see the parsing warning above).
- **`AMI_OPERATOR_QUEUE`** — operator destination hint (used by your transfer
  dialplan; the app passes calls to the `transfer-to-337` context).
- **`AMI_CALL_TIMEOUT`** — seconds the worker waits for a call to complete
  before abandoning it. The DB row then becomes `failed`/`no_rating`.
- **`AMI_RATING_RETRY_LIMIT`** — how many invalid keypresses are tolerated
  before the call gives up asking for a rating.
- **`AMI_RATING_TIMEOUT`** — seconds to wait for rating input (reserved for
  tuning; the dialplan also governs wait time).

## Eskiz SMS

```bash
ESKIZ_EMAIL=<email>
ESKIZ_PASSWORD=<password>
ESKIZ_BASE_URL=https://notify.eskiz.uz/api
ESKIZ_DRY_RUN=false
```

- **`ESKIZ_EMAIL` / `ESKIZ_PASSWORD`** — Eskiz account credentials. The client
  logs in, caches the bearer token, and refreshes it automatically on `401`.
- **`ESKIZ_BASE_URL`** — API base; rarely changed.
- **`ESKIZ_DRY_RUN`** — `true` logs the would-be SMS instead of sending. Great
  for testing the no-rating fallback without spending credit or messaging real
  people.

## Workers

```bash
RIVER_MAX_WORKERS=5
```

- **`RIVER_MAX_WORKERS`** — max concurrent jobs the worker processes. Each
  in-flight call holds one AMI connection and one Asterisk channel, so size this
  against your trunk's channel capacity. See
  [Backups & Scaling](../operations/backups-scaling.md).

## Applying changes

Configuration is read **at startup**. After editing `.env`, restart the
affected process:

```bash
sudo systemctl restart emergency-callback-web emergency-callback-worker
```

There is no live reload.
