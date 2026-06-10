# CLI Commands

The single binary dispatches on its first argument.

```
emergency-callback <command> [args]
```

| Command | Purpose |
|---------|---------|
| `web` | Run the HTTP server (UI + API). |
| `worker` | Run the background job processor (calls, SMS, cleanup). |
| `createuser` | Create a user. |
| `seed` | Insert demo regions/teams. |
| `migrate` | Run application (goose) migrations. |
| `help` | Usage. |

All commands read `.env` from the working directory.

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

!!! note "River migrations are separate"
    `migrate` only manages the **application** schema. The job-queue tables are
    managed by the River CLI: `river migrate-up --database-url "$DATABASE_URL"`.
