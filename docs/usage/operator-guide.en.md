# Operator Guide

Operators have a focused subset of the system: they can **create callbacks** and
**view the callbacks list**. Admin-only areas (dashboard, ratings, teams,
regions, exports) are not accessible — attempting to open them redirects back to
the callbacks list.

## Logging in

Go to `/users/login/` and sign in with your operator credentials (created by an
admin via the CLI). On success you land on the **callbacks list**.

## Creating a callback

1. Open **Новый вызов** (`/callbacks/create/`).
2. Pick a **region** then a **team**.
3. Enter the phone number as `+998901234567`.
4. Submit — the worker begins dialing immediately.

## Viewing callbacks

**Все вызовы** (`/callbacks/`) lists recent callbacks with status, team, rating
(if any), time, and duration. Filters: status, team, region, search, date range.
Open any row for its detail page.

## What you cannot do

| Action | Allowed? |
|--------|----------|
| Create a callback | ✅ |
| View callbacks list & detail | ✅ |
| Dashboard / analytics | ❌ admin only |
| Ratings analytics | ❌ admin only |
| Manage teams / regions | ❌ admin only |
| Excel export | ❌ admin only |

If you need any of the admin functions, ask an administrator. Role assignment is
done when the account is created (`createuser … operator`); only an admin with
server access can change it.
