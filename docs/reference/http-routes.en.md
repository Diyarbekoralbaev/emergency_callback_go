# HTTP Routes

All routes served by `emergency-callback web`. Access is enforced by session
role middleware.

## Access levels

- **public** — no login required.
- **operator** — logged-in operator **or** admin.
- **admin** — logged-in admin only.

Unauthorized access to an admin/operator route redirects to the login page (or
to the callbacks list for wrong-role).

## Authentication

| Method | Path | Access | Purpose |
|--------|------|--------|---------|
| GET | `/users/login/` | public | Login page |
| POST | `/users/login/` | public | Submit credentials (CSRF-protected) |
| GET/POST | `/users/logout/` | logged-in | Log out |

## Callbacks

| Method | Path | Access | Purpose |
|--------|------|--------|---------|
| GET | `/callbacks/` | operator | List with filters |
| GET | `/callbacks/create/` | operator | Create form |
| POST | `/callbacks/create/` | operator | Create a callback (CSRF-protected) |
| GET | `/callbacks/:id/` | operator | Callback detail |
| GET | `/get-teams-by-region/` | operator | AJAX: teams for a region (JSON) |

## Dashboard, ratings, export (admin)

| Method | Path | Access | Purpose |
|--------|------|--------|---------|
| GET | `/` | admin | Dashboard |
| GET | `/ratings/` | admin | Ratings analytics |
| GET | `/export-excel/` | admin | Per-team summary `.xlsx` (honors filters) |

## Teams & regions (admin)

| Method | Path | Access | Purpose |
|--------|------|--------|---------|
| GET | `/teams/` | admin | Team list |
| GET/POST | `/teams/create/` | admin | Create team |
| GET | `/teams/:id/` | admin | Team detail |
| GET/POST | `/teams/:id/edit/` | admin | Edit team |
| GET/POST | `/teams/:id/delete/` | admin | Delete team |
| GET | `/teams/stats-api/` | admin | Team stats (JSON) |
| GET | `/teams/regions/` | admin | Region list |
| GET/POST | `/teams/regions/create/` | admin | Create region |
| GET | `/teams/regions/:id/` | admin | Region detail |
| GET/POST | `/teams/regions/:id/edit/` | admin | Edit region |
| GET/POST | `/teams/regions/:id/delete/` | admin | Delete region |

## Public API & voting

| Method | Path | Access | Purpose |
|--------|------|--------|---------|
| POST | `/api/create/` | public | Create callback via JSON (**CSRF-exempt**) |
| GET | `/vote/:uuid/` | public | SMS vote page |
| POST | `/vote/:uuid/submit/` | public | Submit a vote (**CSRF-exempt**, JSON) |
| GET | `/vote/:uuid/thanks/` | public | Thank-you page |

!!! note "CSRF"
    Browser form POSTs (login, create, team/region edits) are CSRF-protected.
    The machine/SMS endpoints `/api/create/` and `/vote/:uuid/submit/` are
    intentionally CSRF-exempt so external callers and SMS links work. Protect
    `/api/create/` at the network layer if it must not be public.
