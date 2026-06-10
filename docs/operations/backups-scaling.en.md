# Backups & Scaling

## Backups

Everything lives in **one PostgreSQL database** — application data, the job
queue (River), and HTTP sessions. Backing up that database backs up the whole
system state.

```bash
# Dump
pg_dump "$DATABASE_URL" -Fc -f emergency_callback_$(date +%F).dump

# Restore (into an empty database)
pg_restore -d "$DATABASE_URL" --clean --if-exists emergency_callback.dump
```

Recommended:

- Schedule daily `pg_dump` (cron/systemd timer) and keep off-host copies.
- Also version-control your **Asterisk config** (the AMI user, the three custom
  contexts, and the trunk/route changes) and the repo's `audios/` — these are
  needed to rebuild the telephony side.

What is **not** in the database: the binary, `.env` (secrets), Asterisk config,
and the installed audio files. Back those up separately.

## Scaling concurrency

The number of simultaneous calls is governed by **three** limits — the smallest
one wins:

| Limit | Where | Notes |
|-------|-------|-------|
| `RIVER_MAX_WORKERS` | `.env` | Max jobs the worker runs at once. Each in-flight call = one job. |
| DB pool size | `DB_POOL_MAX_CONNS` | Must comfortably exceed the worker count; stay under PostgreSQL `max_connections`. |
| Trunk channels | Asterisk / provider | Your SIP trunk's concurrent-channel cap. |

To raise capacity:

1. Increase `RIVER_MAX_WORKERS`.
2. Ensure `DB_POOL_MAX_CONNS` and PostgreSQL `max_connections` are high enough.
3. Confirm the trunk allows that many concurrent outbound channels.
4. Each call opens a **fresh AMI connection**; make sure Asterisk's AMI
   (`authlimit`) and file-descriptor limits accommodate the peak.

Restart the worker after changing `.env`:

```bash
sudo systemctl restart emergency-callback-worker
```

## Running multiple workers

You can run more than one `worker` process (e.g. on separate hosts) against the
same database — River coordinates job locking in PostgreSQL, so jobs are not
double-processed. Keep the **web** process count modest (it only serves UI and
enqueues). Aggregate worker concurrency still must respect the trunk channel cap.

## Secret rotation

| Secret | Effect of rotating |
|--------|--------------------|
| `SESSION_SECRET` | All users are logged out (sessions invalidated). |
| `CSRF_KEY` | In-flight forms are rejected once (users reload). Must stay exactly 32 bytes. |
| `AMI_SECRET` | Must be changed in **both** `.env` and the Asterisk AMI user, then restart the worker and reload Asterisk. |
| `ESKIZ_PASSWORD` | Update `.env`, restart the worker (the client re-authenticates). |
| DB password | Update the role in PostgreSQL and `DATABASE_URL`, restart both services. |

## Releasing a new build

```bash
# 1. Build
go build -o emergency-callback ./cmd/emergency-callback

# 2. Apply migrations (safe to run repeatedly)
./emergency-callback migrate up
river migrate-up --database-url "$DATABASE_URL"   # only if River added migrations

# 3. Swap the binary and restart
sudo systemctl restart emergency-callback-web emergency-callback-worker
```

In-flight calls finish on the old worker before it exits if you stop it
gracefully; for zero-drop deploys, drain by pausing new callback creation first.

## Monitoring suggestions

- **Liveness:** `curl -sf http://127.0.0.1:8000/users/login/`.
- **Worker progress:** watch `journalctl -u emergency-callback-worker`.
- **Stuck calls:** the periodic `CleanupStaleCalls` finalizes anything stuck
  >30 min; a rising count of `failed`/`no_rating` indicates a trunk problem.
- **Queue depth:** inspect River's tables in PostgreSQL (e.g. count rows in
  `river_job` by state) to see backlog.
