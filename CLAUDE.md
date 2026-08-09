# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

Emergency ambulance callback + rating system for Uzbekistan (Karakalpakstan). An operator
creates a callback request in a web panel; a background worker originates an Asterisk/FreePBX
call, plays Karakalpak prompts, collects a 1–5 DTMF rating, optionally transfers to a live
operator, and falls back to an SMS vote link if no rating was captured.

Go rewrite of a Django + Celery app. Single binary, PostgreSQL-only (no Redis). URL paths,
DB schema, dialplan context names, and audio prompt names are deliberately preserved 1:1 from
the Django original — many `// Mirrors callbacks/…py` comments reference it.

Two user-facing languages: Russian for the admin/operator UI, Karakalpak for callee-facing
strings (SMS body, vote page). Keep that split when editing strings.

## Commands

```bash
# Build (dev)
go build -o emergency-callback ./cmd/emergency-callback

# Build (production, ~30MB)
CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath -o emergency-callback ./cmd/emergency-callback

# Run — TWO processes, both required for the system to function
./emergency-callback web        # HTTP server (enqueues jobs only)
./emergency-callback worker     # River workers (actually places calls / sends SMS)

# Other subcommands
./emergency-callback migrate <up|down|status|reset|version>
./emergency-callback createuser <username> <password> [admin|operator]
./emergency-callback seed       # demo regions/teams; requires an existing admin

# Tests (only internal/auth/password_test.go exists today)
go test ./...
go test ./internal/auth -run TestHashPassword -v

go vet ./...

# Regenerate DB layer after editing internal/db/queries/*.sql or migrations/
sqlc generate        # config: sqlc.yaml → internal/db/sqlc/

# Docs site (MkDocs Material, bilingual ru/en)
python -m venv .docs-venv && . .docs-venv/bin/activate && pip install -r docs/requirements.txt
mkdocs serve
mkdocs build                        # → site/
ENABLE_PDF_EXPORT=1 mkdocs build    # → site/pdf/
```

## Deployment

`install.sh` (run as root from the repo root) is the canonical deployment path and is
**idempotent** — re-run it to redeploy rather than hand-editing a live install. It installs
Go/PostgreSQL/River CLI if missing, creates the role + DB with ownership grants, generates all
secrets, writes `.env`, builds, migrates, creates the admin user, installs and restarts the two
systemd units (`emergency-callback-web`, `emergency-callback-worker`), and writes
`INSTALL_CREDENTIALS.txt` (chmod 600). `--yes` for zero-touch.

FreePBX is assumed to be a **separate server** — the installer never touches it. It emits
`freepbx-bundle/` (AMI user config, dialplan, WAV prompts, README) to apply there manually.

`uninstall.sh` removes services and generated files; `--drop-db` / `--purge` also drop the
database and role.

Deployment facts that are easy to get wrong:

- **Working directory matters.** `templates/` (via `templates.Load("templates")`) and
  `migrations/` (goose, overridable with `MIGRATIONS_DIR`) are read from disk relative to the
  process CWD — nothing is embedded. systemd units must set `WorkingDirectory` to the repo root,
  and `templates/` + `migrations/` + `audios/` must ship alongside the binary.
- **Two migration systems.** `./emergency-callback migrate up` applies goose migrations
  (app tables + the scs `sessions` table). River's job-queue tables need the separate CLI:
  `river migrate-up --database-url "$DATABASE_URL"`. Both are idempotent.
- **Go version mismatch in the installer.** `go.mod` declares `go 1.25.7`, but `install.sh`'s
  `ensure_go` fallback installs Go 1.23.0. On a host without Go the build will fail — bump that
  version, or install a newer Go first.
- **CSRF trusted origins are hardcoded** in `internal/server/routes.go` (`csrf.TrustedOrigins`,
  currently `103tezjardem.uz`, `callback.diyarbek.uz`, localhost:9999). A new public domain
  requires a code change, not just `SITE_DOMAIN`.
- **Plain HTTP is assumed.** `csrf.Secure(false)`, `sm.Cookie.Secure = false`, and
  `csrf.PlaintextHTTPRequest` wrapping — designed for TLS terminated at a reverse proxy. Flip
  both `Secure` flags if serving HTTPS directly.
- **No static file route.** `static/` is gitignored/empty; templates pull Bootstrap 5 from a CDN.
- `.env` is loaded via godotenv from CWD. `config.Load()` **panics** on a missing required var
  (`DATABASE_URL`, `SESSION_SECRET`, `CSRF_KEY`, `AMI_HOST`, `AMI_USERNAME`, `AMI_SECRET`).
- `ESKIZ_DRY_RUN=true` (or a blank `ESKIZ_EMAIL`) logs SMS instead of sending.

## Architecture

### Process split

Both processes are the same binary sharing one Postgres pool.

- `web` (`cmd/emergency-callback/web.go`) — builds the Gin router and a River client with
  `queueOnly=true`: **no workers registered**, insert-only. If the worker process isn't running,
  callbacks are created and jobs queue up forever with no calls placed.
- `worker` (`worker.go`) — `queueOnly=false`: registers the three workers plus the periodic
  cleanup job, then blocks on the signal context.

Job insertion is transactional: `s.withTx(...)` inserts the callback row and calls
`River.InsertTx` in the same pgx transaction, so a job never references a non-existent callback.

### Job pipeline (`internal/jobs/`)

| Job kind | Trigger | Does |
|---|---|---|
| `process_callback_call` | callback created (web form or `POST /api/create/`) | runs the AMI bridge for one call; on no rating, enqueues the SMS job |
| `send_rating_sms` | no DTMF rating, or cleanup found a stale call | Eskiz SMS with a `SITE_DOMAIN/vote/<uuid>` link |
| `cleanup_stale_calls` | periodic, every 15 min | callbacks stuck >30 min → `failed`/`no_rating`/`transferred`, then enqueue SMS |

### AMI bridge (`internal/ami/`) — the heart of the system

`Bridge.Run` owns one TCP connection for the lifetime of one call and blocks until hangup or
`AMI_CALL_TIMEOUT`. Flow:

1. `Originate` to `Local/<phone>@from-internal/n` into the `ambulance-callback` context. The
   `/n` (no optimization) is **required** so both Local legs survive bridging — otherwise the
   app can't `Redirect` the application leg afterwards.
2. Channel variables use the `__` prefix (`__CALL_ID`, `__PHONE_NUMBER`, `__BRIGADE_ID`,
   `__CALLBACK_REQUEST_ID`) so they inherit to every child channel and `${CALL_ID}` resolves in
   the dialplan everywhere.
3. `UserEvent(CallAnswered)` identifies the authoritative channel — it overrides whatever
   `Newchannel` guessed, because the two Local legs race.
4. Audio is played by `Redirect`ing the channel into the `play-audio` context at an extension
   from `ami.AudioMap` (`ambulance-rating-request`, `-thankyou`, `-invalid`, `ambulance-transfer-
   message`, `-error`). The WAVs live in `audios/` and must be installed on the FreePBX box.
5. DTMF drives the state machine in `events.go:handleDTMF` — states in `state.go`:
   `dialing → answered → waiting_rating → rating_received → waiting_transfer_decision →
   transferring/completed`. Rating 1–5 is saved immediately; then `0` or `9` transfers.

Two subtleties that will bite anyone editing `events.go`:

- **`handleDTMF` runs on the AMI message loop and must not block.** A `time.Sleep` there
  queues hangup/DTMF events behind it (this previously truncated the thank-you audio). The one
  remaining exception is `handleInvalidRating`, which does sleep — treat that as legacy.
- **DTMF arrives multiple times.** The same keypress surfaces on the callee's PJSIP leg, Local
  `;1`, and Local `;2`. `onDTMFEnd` matches a digit to the call by uniqueid/linkedid/channel/
  phone-substring, and a 4-second `dtmfDedupeWindow` drops the echo — without it, the rating
  digit gets re-read as the transfer choice.

`transferToOperator` hardcodes the dialplan context `"transfer-to-337"`. `AMI_OPERATOR_QUEUE`
is parsed into config but currently unused — changing the operator target means editing the
FreePBX dialplan, not the env var.

`formatPhoneNumber` strips non-digits and drops a leading `998` from 12-digit numbers; the
FreePBX outbound route must match the resulting 9-digit form.

### Web layer

- `internal/server/routes.go` — the whole route table in one place, plus `registerURLs()`
  which maps Django-style names (`callbacks:list`) to paths for the `urlFor` template func.
  Handler chain: `scs.LoadAndSave` → CSRF dispatcher → Gin. `/api/create/` and
  `/vote/*/submit/` are routed *around* CSRF entirely (see `isCSRFExempt`).
- Access tiers: public (login, vote pages, `/api/create/`), operator+admin (`/callbacks/…`),
  admin-only (dashboard `/`, `/ratings/`, `/export-excel/`, `/teams/…`).
- `internal/handlers/server.go` holds the shared `Server` struct plus `baseData` (session user +
  CSRF field + flash), `render` / `renderStandalone`, and `withTx`.
- `internal/templates/loader.go` — two template families: **layout** pages parsed together with
  `base.html` and rendered via `ExecuteTemplate(w, "base", …)`, and **standalone** pages
  (basename starting with `vote`) rendered on their own. Adding a full-page template that
  shouldn't inherit the admin chrome means naming it `vote*` or extending the loader rule.
- Sessions: scs backed by the Postgres `sessions` table (migration 0007), cookie `ecb_session`,
  7-day lifetime. Passwords are bcrypt cost 12 — **not** Django-compatible hashes.

### Data layer

`internal/db/queries/*.sql` → `sqlc generate` → `internal/db/sqlc/*.sql.go` (pgx/v5, pointers
for nullable columns, JSON tags). Never hand-edit files in `internal/db/sqlc/`. Table names keep
the Django prefixes (`callbacks_callbackrequest`, `teams_team`, `regions_region`, `ratings_…`).

Timestamps are stored UTC and displayed Asia/Tashkent — always go through `internal/tz`
(`FromPGTimestamp`, `DayBoundsTashkentUTC`) rather than formatting a `pgtype.Timestamptz`
directly, or dashboard "today" filters will be off by five hours.

### Docs

`docs/` is a bilingual MkDocs site: un-suffixed `.md` is Russian (default), `.en.md` is English.
Adding a page means adding both files plus a `nav:` entry and, if the title is new, a
`nav_translations` entry in `mkdocs.yml`. `site/` and `.docs-venv/` are build artifacts and
gitignored.
