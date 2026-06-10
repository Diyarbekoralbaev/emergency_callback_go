# Admin Guide

Admins have full access: the dashboard, ratings analytics, team/region
management, Excel export, and everything operators can do. The admin UI is in
**Russian**.

Log in at `/users/login/`. After login you land on the dashboard.

## Navigation (sidebar)

| Item | Path | Who sees it |
|------|------|-------------|
| Панель управления (Dashboard) | `/` | admin |
| Новый вызов (New callback) | `/callbacks/create/` | admin, operator |
| Все вызовы (All callbacks) | `/callbacks/` | admin, operator |
| Регионы (Regions) | `/teams/regions/` | admin |
| Бригады (Teams) | `/teams/` | admin |
| Оценки (Ratings) | `/ratings/` | admin |

## Dashboard (`/`)

A filtered overview of activity:

- **Filters:** region, team, date range. Defaults to today.
- **Call stats:** total, completed (incl. transferred), failed, no-rating.
- **Rating stats:** total ratings, average, success/failure rate, 1–5
  distribution.
- **Team/region performance** and a **recent calls** list.
- **Excel export** (`/export-excel/`) — downloads a per-team summary spreadsheet
  honoring the current filters.

## Creating a callback (`/callbacks/create/`)

1. Choose a **region**, then a **team**.
2. Enter the **phone number** in international form, e.g. `+998901234567`.
3. Submit. The system creates the record and the worker starts dialing
   immediately.

!!! tip "Phone number format"
    Enter `+998…`. The system normalizes it (strips the `998` country code)
    before handing it to the dialplan. See
    [Call Flow](../telephony/call-flow.md).

The callback appears in **Все вызовы** with a live status.

## Callbacks list (`/callbacks/`)

- Filter by status, team, region, search (phone/team), and date range.
- Each row shows phone, team/region, status badge, rating stars (if any),
  created time, and duration.
- Click a row to open the **detail** page (`/callbacks/<id>/`) with full timing,
  the rating (if collected), error message (if failed), and the SMS vote URL.

Status meanings are in [Call Status](../reference/call-status.md).

## Ratings (`/ratings/`)

Analytics over collected ratings: totals, average, the share of "good" (4–5★)
ratings, and a filterable list (region, team, rating value, date). Use it to
review service quality by team/region over a period.

## Regions & Teams

The dispatch hierarchy is **Region → Team**. Every callback is attached to a
team, and a team belongs to a region.

=== "Regions (`/teams/regions/`)"

    - **List** shows code, name, active/total team counts, status.
    - **Create / Edit** set name, code, description, active flag.
    - **Detail** lists the teams in the region.
    - **Delete** removes the region (and, by cascade, its teams — confirm
      carefully).

=== "Teams (`/teams/`)"

    - **List** with search and region filter; toggle showing inactive teams.
    - **Create / Edit** set name, region, description, active flag.
    - Inactive teams are excluded from new callbacks and dashboards.

## Users & roles

- **admin** — everything above.
- **operator** — can create callbacks and view the callbacks list only.

Users are created from the command line (there is no in-app user management
screen):

```bash
./emergency-callback createuser <username> <password> [admin|operator]
```

See [CLI Commands](../reference/cli-commands.md).
