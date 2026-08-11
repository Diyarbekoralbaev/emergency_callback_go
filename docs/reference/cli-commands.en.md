# CLI Commands

The single binary dispatches on its first argument.

```
emergency-callback <command> [args]
```

| Command | Purpose |
|---------|---------|
| `setup` | **Interactive install wizard** (or `--non-interactive` with `SETUP_*`). |
| `doctor` | Health checks (read-only): DB, migrations, AMI/ARI, Eskiz, sox, web. |
| `web` | Run the HTTP server (UI + API). |
| `worker` | Run the background job processor (calls, SMS, cleanup). |
| `createuser` | Create a user. |
| `seed` | Insert demo regions/teams. |
| `migrate` | App schema **and** River job-queue migrations (external `river` CLI no longer needed). |
| `docs` | Serve the built docs site (`site/`). |
| `version` | Binary version. |
| `help` | Usage. |

All commands read `.env` from the working directory. Templates, migrations and
audio files are **embedded in the binary**: when missing on disk the embedded
copy is used (a directory next to the binary takes precedence — handy when
running from a cloned repo).

---

## `setup`

```bash
sudo ./emergency-callback setup                    # interactive
sudo ./emergency-callback setup --non-interactive  # values from SETUP_<KEY>
```

Detects the environment (OS, systemd, PostgreSQL, port, sox), asks questions
with options where reality deviates (busy port, passworded Postgres, remote
DB, no systemd…), shows a plan, then applies it: DB provisioning, `.env` merge
(existing values and secrets are **preserved**), migrations, admin user, audio
(optional SSH copy to the PBX), systemd units (or `run-*.sh` without systemd),
health check. Idempotent — safe to re-run.

## `doctor`

```bash
./emergency-callback doctor
```

Read-only PASS/WARN/FAIL table: `.env`, config, DB + migrations, AMI login,
Eskiz auth, sox, WAV files, web server response, unit states. Exit code 1 on
any FAIL.

---

## `web`

```bash
./emergency-callback web
```

Starts the HTTP server on `HTTP_ADDR`. Connects to the database and to River
(in queue-only mode — it enqueues jobs but does not process them). Run under
systemd in production ([Running the Services](../operations/running-services.md)).

## `worker`

```bash
./emergency-callback worker
```

Starts the River worker: registers the three job types and the periodic
cleanup, then processes jobs until stopped. **Required** for any call or SMS to
actually happen. Concurrency = `RIVER_MAX_WORKERS`.

Jobs:

- `ProcessCallback` — drives one call over AMI.
- `SendRatingSMS` — sends the fallback SMS.
- `CleanupStaleCalls` — periodic (every 15 min) finalizer for stuck calls.

## `createuser`

```bash
./emergency-callback createuser <username> <password> [admin|operator]
```

Creates a user with a bcrypt-hashed password. Role defaults to `operator` if
omitted. `admin` also sets `is_staff`/`is_superuser`.

```bash
./emergency-callback createuser admin 'StrongPass!' admin
./emergency-callback createuser dispatcher1 'StrongPass!' operator
```

## `seed`

```bash
./emergency-callback seed
```

Inserts two demo regions and four demo teams (owned by an existing admin). Run
once after creating an admin, for a quick demo/test dataset. Safe to skip in
production.

## `migrate`

```bash
./emergency-callback migrate <up|down|status|version|reset>
```

| Subcommand | Effect |
|-----------|--------|
| `up` | Apply all pending migrations. |
| `down` | Roll back the last migration. |
| `status` | Show applied/pending migrations. |
| `version` | Show current schema version. |
| `reset` | Roll back all migrations (**destructive**). |

Override the migrations directory with `MIGRATIONS_DIR` if needed.

!!! note "River migrations are built in"
    `migrate up` applies **both** migration systems: the application schema
    (goose) and the River job-queue tables (in-process). No external `river`
    CLI is needed. `down`/`reset` touch only the application schema.
