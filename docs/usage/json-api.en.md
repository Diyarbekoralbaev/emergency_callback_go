# JSON API

A single public endpoint lets external systems create a callback
programmatically (e.g. a dispatch system firing a callback right after an
ambulance run).

## Create a callback

```
POST /api/create/
Content-Type: application/json
```

Request body:

```json
{ "phone_number": "+998901234567" }
```

| Field | Required | Notes |
|-------|----------|-------|
| `phone_number` | yes | International form (`+998…`). |

### Behavior

- A **random active team** is assigned automatically.
- The callback is created and a `ProcessCallback` job is enqueued in the same DB
  transaction; the worker starts dialing immediately.
- The endpoint is **CSRF-exempt** and requires **no authentication** — protect
  it at the network layer (firewall / reverse proxy allowlist) if it must not be
  public.

### Success response

```json
{
  "success": true,
  "callback_id": 123,
  "phone_number": "+998901234567",
  "team": "Бригада 2",
  "region": "Каракалпакстан",
  "status": "pending",
  "message": "..."
}
```

### Error responses

| HTTP | Body | Cause |
|------|------|-------|
| 400 | `{"error": "..."}` | Missing/invalid `phone_number` or malformed JSON. |
| 500 | `{"error": "..."}` | No active teams, or a server/database error. |

## Example

```bash
curl -X POST http://127.0.0.1:8000/api/create/ \
  -H 'Content-Type: application/json' \
  -d '{"phone_number":"+998901234567"}'
```

## Notes

- For a specific team (rather than random), create the callback through the admin
  UI form instead — the API picks a random active team by design.
- The same call lifecycle applies as for UI-created callbacks; see
  [Call Flow](../telephony/call-flow.md).
