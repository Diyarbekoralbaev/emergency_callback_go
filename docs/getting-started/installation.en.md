# Installation

This walks a **fresh server** to a running application. Telephony wiring is a
separate step — see [FreePBX Integration](../telephony/freepbx-integration.md).

## Quick automated install (recommended)

Installation is driven by the **built-in wizard** `emergency-callback setup` —
interactive, idempotent (safe to re-run), with checks at every step and
options for when something deviates from the norm (a busy port, an already
installed PostgreSQL with a password, a remote database, no systemd…).
`install.sh` is just a thin bootstrap: it downloads the prebuilt binary from
GitHub Releases (or builds from source when Go is present) and launches the
wizard.

```bash
git clone <your-repo-url> emergency_callback_go
cd emergency_callback_go
sudo ./install.sh              # binary + setup wizard
# force downloading a release:     sudo ./install.sh --download
# force building from source:      sudo ./install.sh --build
```

The wizard: detects the environment → asks questions (Enter = a sensible
default; an existing `.env` is **not clobbered**, secrets are preserved) →
shows a plan → applies it: PostgreSQL, `.env`, migrations (goose + River,
embedded), the admin user, audio files (over SSH to the PBX if desired),
systemd units → verifies the web server responds. Credentials go to
`INSTALL_CREDENTIALS.txt` (chmod 600).

Verify at any time:

```bash
./emergency-callback doctor
```

For automation (CI/Ansible): `setup --non-interactive` with values in
`SETUP_<KEY>` variables (e.g. `SETUP_DB_NAME`, `SETUP_AMI_HOST`); missing
required values abort cleanly with a list — nothing is applied halfway.

!!! info "FreePBX is a separate server"
    The script does **not** touch FreePBX. It writes a `freepbx-bundle/` folder
    (AMI user, dialplan, audio files, and a step-by-step README) that you apply
    on your FreePBX server. See
    [FreePBX Integration](../telephony/freepbx-integration.md).

!!! warning "Secrets"
    `install.sh` generates passwords/secrets and writes them to
    `INSTALL_CREDENTIALS.txt` (and prints a summary at the end). Keep that file
    safe; it, `.env`, and `freepbx-bundle/` are git-ignored.

After the script finishes, open the web panel and continue with
[FreePBX Integration](../telephony/freepbx-integration.md). The rest of this page
documents the same steps **manually** if you prefer to run each yourself.

---

## Manual installation

Do these in order.

## 1. Get the code and build

```bash
git clone <your-repo-url> emergency_callback_go
cd emergency_callback_go

go build -o emergency-callback ./cmd/emergency-callback
./emergency-callback help
```

For a smaller binary to ship to production:

```bash
CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath -o emergency-callback ./cmd/emergency-callback
```

Repository layout:

```
cmd/emergency-callback/   entrypoint + subcommands (web, worker, createuser, seed, migrate)
internal/                 application code (ami, auth, db, handlers, jobs, sms, …)
migrations/               goose SQL migrations (the database schema)
templates/                HTML templates (Bootstrap 5 via CDN)
audios/                   the 6 voice-prompt WAV files for Asterisk
docs/                     this documentation
.env.example              configuration template
```

## 2. Create the PostgreSQL database

```bash
sudo -u postgres psql -c "CREATE USER ecb WITH PASSWORD 'CHANGE_ME_STRONG';"
sudo -u postgres psql -c "CREATE DATABASE emergency_callback OWNER ecb;"
sudo -u postgres psql -d emergency_callback -c "GRANT ALL ON SCHEMA public TO ecb;"
```

!!! warning "PostgreSQL 15+: privileges on the `public` schema"
    Since PostgreSQL 15, owning a database does **not** automatically grant
    `CREATE` on the `public` schema. Without the third command above, migrations
    (goose and River, both inside `migrate up`) fail with
    `permission denied for schema public (SQLSTATE 42501)`. The `GRANT` fixes it.

The connection string you will use:

```
postgres://ecb:CHANGE_ME_STRONG@127.0.0.1:5432/emergency_callback?sslmode=disable
```

!!! tip "Production TLS"
    Prefer `sslmode=require` against a TLS-enabled PostgreSQL in production.

## 3. Configure `.env`

```bash
cp .env.example .env
$EDITOR .env
```

Fill in at least `DATABASE_URL`, `SESSION_SECRET`, `CSRF_KEY`, the `AMI_*`
values, and the `ESKIZ_*` values. Every variable is explained in
[Configuration](configuration.md).

Generate the secrets:

```bash
openssl rand -base64 32   # SESSION_SECRET
openssl rand -base64 24   # CSRF_KEY  (decodes to exactly 32 bytes)
```

!!! warning "`.env` parsing"
    Values must be plain `KEY=value`. Do **not** include unquoted `<`, `>`, or
    spaces in a value — that breaks the dotenv parser and the app will report a
    missing variable on startup. For example use `AMI_CALLER_ID=781138081`, not
    `AMI_CALLER_ID="Service" <781138081>`.

## 4. Run database migrations

Two independent migration sets run against the **same** database.

### 4a. Application schema

```bash
./emergency-callback migrate up
```

Creates `users`, `teams_region`, `teams_team`, `callbacks_callbackrequest`,
`callbacks_rating`, `sessions`, and the `pgcrypto` extension. (Schema details:
[Database Schema](../reference/database-schema.md).)

### 4b. Job-queue (River) tables

As of the current version, `migrate up` applies these itself, in-process —
the external `river` CLI is not needed. There is no separate step anymore.

## 5. Create the first admin user

```bash
# createuser <username> <password> [admin|operator]
./emergency-callback createuser admin 'CHANGE_ME' admin
```

Optionally seed demo regions and teams (an admin must exist first):

```bash
./emergency-callback seed
```

## 6. Start it (quick check)

```bash
./emergency-callback web      # terminal 1 — HTTP server
./emergency-callback worker   # terminal 2 — background jobs
```

Visit `http://<server>:8000/users/login/` and log in. For a production setup
with systemd and a TLS proxy, see
[Running the Services](../operations/running-services.md).

## 7. Wire up telephony

The app can now create callbacks, but it cannot place calls until Asterisk is
configured. Continue with
[FreePBX Integration](../telephony/freepbx-integration.md).

---

## Installation checklist

- [ ] Binary builds (`./emergency-callback help` works)
- [ ] PostgreSQL role + database created
- [ ] `.env` filled in; secrets generated
- [ ] `migrate up` succeeded
- [ ] Admin user created
- [ ] `web` + `worker` start without errors
- [ ] FreePBX integration done (next page)
