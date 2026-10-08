# Slots API

Create users and time slots; hold, book, cancel and reschedule slots; and see each user's booking history.

- **Base URL (local):** `http://localhost:4500`
- **Interactive docs:** `http://localhost:4500/docs` (Swagger UI)
- **OpenAPI spec:** `http://localhost:4500/openapi.yaml` (also [`openapi.yaml`](openapi.yaml) in this folder)

## Conventions

- All request and response bodies are JSON. Send `Content-Type: application/json`.
- **Authentication:** none yet. The acting user is passed as `userId` in the request body.
- **Timestamps** are RFC 3339, e.g. `2026-10-10T10:00:00Z`. Responses may show the same
  instant in the server's time zone (e.g. `2026-10-10T15:30:00+05:30`), so parse them as
  dates rather than comparing strings.
- **Errors** always have this shape:
  ```json
  { "error": "<message>" }
  ```
- **CORS** is not enabled yet; a browser UI on a different origin will be blocked.

## Holds

A user can put a free slot **on hold** (`status: 50`) while they decide, then confirm it by
booking or let it go by cancelling.

- A hold lasts **`booking.holdMinutes`** from the server config (`config/local.yaml`, default
  file sets `10`), counted from the slot's last update. Active holds include
  `holdExpiresAt` so the UI can show a countdown.
- When a hold expires, the API shows the slot as **not booked** (`status: 0`, no `bookedBy`)
  everywhere slots are returned, and anyone can hold or book it. The database row is not
  rewritten; expiry is applied when the slot is read or changed.
- While a hold is active, only the holder can act on the slot: they can
  [book](#post-slotsidbook) it to confirm, or [cancel](#post-slotsidcancel) to release it.
  Everyone else gets `409 slot is on hold`.
- A hold can't be extended by holding again; that also returns `409 slot is on hold`.
- Expiry is not written to history; a hold that runs out leaves only its `held` entry.

Typical flow: `POST /slots/{id}/hold` → show countdown from `holdExpiresAt` →
`POST /slots/{id}/book` (confirm) or `POST /slots/{id}/cancel` (release).

## Endpoints

| Method | Path | Description |
|---|---|---|
| GET | [`/health`](#get-health) | Health check |
| POST | [`/users`](#post-users) | Create a user |
| GET | [`/users/{id}`](#get-usersid) | Get a user |
| GET | [`/users/{id}/history`](#get-usersidhistory) | A user's booking history |
| GET | [`/slots`](#get-slots) | List all slots |
| POST | [`/slots`](#post-slots) | Create a slot |
| POST | [`/slots/bulk`](#post-slotsbulk) | Create several slots at once |
| GET | [`/slots/{id}`](#get-slotsid) | Get a slot |
| POST | [`/slots/{id}/hold`](#post-slotsidhold) | Put a slot on hold |
| POST | [`/slots/{id}/book`](#post-slotsidbook) | Book a slot (or confirm your hold) |
| POST | [`/slots/{id}/cancel`](#post-slotsidcancel) | Cancel a booking or release your hold |
| POST | [`/slots/{id}/reschedule`](#post-slotsidreschedule) | Move a booking to another slot |

---

## Data types

### User

| Field | Type | Notes |
|---|---|---|
| `id` | integer | |
| `name` | string | max 255 |
| `email` | string | max 255, unique, stored lower-cased |

```json
{ "id": 1, "name": "Asha", "email": "asha@example.com" }
```

### Slot

| Field | Type | Notes |
|---|---|---|
| `id` | integer | |
| `date` | timestamp | Calendar day, returned as midnight UTC, e.g. `2026-10-10T00:00:00Z` |
| `from` | timestamp | Start time |
| `to` | timestamp | End time |
| `status` | integer | `0` = not booked, `50` = on hold, `100` = booked. An expired hold is shown as `0` |
| `bookedBy` | integer | User id of the booker or holder. **Omitted** when the slot is not booked |
| `createdAt` | timestamp | |
| `updatedAt` | timestamp | |
| `holdExpiresAt` | timestamp | When the hold ends. **Only present** while `status` is `50` |

```json
{
  "id": 1,
  "date": "2026-10-10T00:00:00Z",
  "from": "2026-10-10T10:00:00Z",
  "to": "2026-10-10T11:00:00Z",
  "status": 100,
  "bookedBy": 2,
  "createdAt": "2026-10-08T12:10:02Z",
  "updatedAt": "2026-10-08T12:10:05Z"
}
```

A slot on hold:
```json
{
  "id": 1,
  "date": "2026-10-10T00:00:00Z",
  "from": "2026-10-10T10:00:00Z",
  "to": "2026-10-10T11:00:00Z",
  "status": 50,
  "bookedBy": 2,
  "createdAt": "2026-10-08T12:10:02Z",
  "updatedAt": "2026-10-08T12:10:05Z",
  "holdExpiresAt": "2026-10-08T12:20:05Z"
}
```

### SlotHistory

One booking change made by a user.

| Field | Type | Notes |
|---|---|---|
| `id` | integer | |
| `userId` | integer | Who made the change |
| `slotId` | integer | The slot acted on. For a reschedule, the slot moved **to** |
| `previousSlotId` | integer | For a reschedule, the slot moved **from**. **Omitted** otherwise |
| `action` | integer | `1` = booked, `2` = unbooked (cancelled or hold released), `3` = rescheduled, `4` = held |
| `createdAt` | timestamp | When the change happened |
| `slot` | [Slot](#slot) | The slot `slotId`, as it is **now** |
| `previousSlot` | [Slot](#slot) | The slot `previousSlotId`, as it is **now**. Only for reschedules |

> `slot` and `previousSlot` show each slot's **current** state, not its state at the time of
> the change. Use `action` to know what happened, not the embedded slot's `status`.

Suggested UI labels:

| `action` | Label |
|---|---|
| `1` | Booked *slot.from – slot.to* |
| `2` | Cancelled *slot.from – slot.to* |
| `3` | Rescheduled from *previousSlot.from* to *slot.from* |
| `4` | Held *slot.from – slot.to* |

---

## Health

### GET /health

**200 OK**
```json
{ "status": "ok" }
```

---

## Users

### POST /users

Create a user. `name` is trimmed; `email` is trimmed and lower-cased.

**Request**
```json
{ "name": "Asha", "email": "asha@example.com" }
```

| Field | Type | Required |
|---|---|---|
| `name` | string | yes |
| `email` | string (valid email) | yes |

**Responses**

| Status | When | Body |
|---|---|---|
| 201 | Created | [User](#user) |
| 400 | Missing field or invalid email | `{"error": "email is invalid"}` |
| 409 | Email already used | `{"error": "record already exists"}` |

### GET /users/{id}

**Responses**

| Status | When | Body |
|---|---|---|
| 200 | Found | [User](#user) |
| 400 | `id` is not a positive integer | `{"error": "invalid id"}` |
| 404 | No such user | `{"error": "record not found"}` |

### GET /users/{id}/history

The user's booking history, **newest first**. Every successful book, cancel and reschedule
is recorded; failed attempts are not.

**Query parameters** (both optional)

| Param | Default | Notes |
|---|---|---|
| `limit` | `50` | 1–200 |
| `offset` | `0` | Entries to skip, for paging |

Example: `GET /users/1/history?limit=20&offset=0`

**200 OK** — array of [SlotHistory](#slothistory); `[]` if there are none.

```json
[
  {
    "id": 4,
    "userId": 1,
    "slotId": 2,
    "action": 2,
    "createdAt": "2026-10-08T12:33:52Z",
    "slot": { "id": 2, "date": "2026-10-10T00:00:00Z", "from": "2026-10-10T11:00:00Z", "to": "2026-10-10T12:00:00Z", "status": 0, "createdAt": "...", "updatedAt": "..." }
  },
  {
    "id": 2,
    "userId": 1,
    "slotId": 2,
    "previousSlotId": 1,
    "action": 3,
    "createdAt": "2026-10-08T12:33:51Z",
    "slot": { "id": 2, "from": "2026-10-10T11:00:00Z", "to": "2026-10-10T12:00:00Z", "...": "..." },
    "previousSlot": { "id": 1, "from": "2026-10-10T10:00:00Z", "to": "2026-10-10T11:00:00Z", "...": "..." }
  },
  {
    "id": 1,
    "userId": 1,
    "slotId": 1,
    "action": 1,
    "createdAt": "2026-10-08T12:33:50Z",
    "slot": { "id": 1, "from": "2026-10-10T10:00:00Z", "to": "2026-10-10T11:00:00Z", "...": "..." }
  }
]
```
This reads: booked slot 1 → rescheduled from slot 1 to slot 2 → cancelled slot 2.

**Errors**

| Status | When | Body |
|---|---|---|
| 400 | `id` not a positive integer | `{"error": "invalid id"}` |
| 400 | `limit`/`offset` not an integer | `{"error": "invalid limit"}` / `{"error": "invalid offset"}` |
| 400 | `limit` outside 1–200 | `{"error": "limit must be between 1 and 200"}` |
| 400 | negative `offset` | `{"error": "offset must not be negative"}` |
| 404 | No such user | `{"error": "record not found"}` |

---

## Slots

### GET /slots

List all slots, ordered by `date` then `from`. Returns `[]` when there are none.

**200 OK** — array of [Slot](#slot)

### GET /slots/{id}

**Responses**

| Status | When | Body |
|---|---|---|
| 200 | Found | [Slot](#slot) |
| 400 | `id` is not a positive integer | `{"error": "invalid id"}` |
| 404 | No such slot | `{"error": "record not found"}` |

### POST /slots

Create a slot. New slots are always unbooked (`status: 0`, no `bookedBy`).

**Request**
```json
{
  "date": "2026-10-10",
  "from": "2026-10-10T10:00:00Z",
  "to": "2026-10-10T11:00:00Z"
}
```

| Field | Type | Required | Notes |
|---|---|---|---|
| `date` | string | yes | `YYYY-MM-DD` |
| `from` | string | yes | RFC 3339; must be before `to` |
| `to` | string | yes | RFC 3339 |

**Responses**

| Status | When | Body |
|---|---|---|
| 201 | Created | [Slot](#slot) |
| 400 | Invalid input | e.g. `{"error": "from must be before to"}`, `{"error": "date must be in YYYY-MM-DD format"}` |

### POST /slots/bulk

Create several slots in one request. **All-or-nothing:** if any slot is invalid, none are created.

**Request**
```json
{
  "slots": [
    { "date": "2026-10-11", "from": "2026-10-11T10:00:00Z", "to": "2026-10-11T11:00:00Z" },
    { "date": "2026-10-11", "from": "2026-10-11T11:00:00Z", "to": "2026-10-11T12:00:00Z" }
  ]
}
```

`slots` must contain at least one item; each item has the same fields as [POST /slots](#post-slots).

**Responses**

| Status | When | Body |
|---|---|---|
| 201 | Created | array of [Slot](#slot), in request order |
| 400 | Any item invalid | Names the failing item, e.g. `{"error": "slots[1]: from must be before to"}` |

### POST /slots/{id}/hold

Put a slot on hold for a user for `booking.holdMinutes` (see [Holds](#holds)). The slot must
be free or have an expired hold.

**Request**
```json
{ "userId": 2 }
```

**Responses**

| Status | When | Body |
|---|---|---|
| 200 | Held | [Slot](#slot) with `status: 50`, `bookedBy` and `holdExpiresAt` set |
| 400 | Bad `id`, or `userId` missing / not a positive integer | `{"error": "invalid id"}`, or a [field-validation message](#error-reference) |
| 404 | Slot or user doesn't exist | `{"error": "record not found"}` |
| 409 | Slot is booked | `{"error": "slot is already booked"}` |
| 409 | Slot has an active hold (anyone's, including yours) | `{"error": "slot is on hold"}` |
| 409 | Slot changed during the request | `{"error": "slot was modified concurrently, retry"}` |

> **Concurrency:** if several users hold the same slot at once, exactly one gets `200`; the
> others get `409 slot is on hold`.

### POST /slots/{id}/book

Book a slot for a user. Works on a free slot, a slot whose hold has expired, or a slot the
user is holding (this confirms the hold).

**Request**
```json
{ "userId": 2 }
```

**Responses**

| Status | When | Body |
|---|---|---|
| 200 | Booked | [Slot](#slot) with `status: 100` and `bookedBy` set |
| 400 | Bad `id`, or `userId` missing / not a positive integer | `{"error": "invalid id"}`, or a [field-validation message](#error-reference) |
| 404 | Slot or user doesn't exist | `{"error": "record not found"}` |
| 409 | Already booked | `{"error": "slot is already booked"}` |
| 409 | Another user has an active hold | `{"error": "slot is on hold"}` |
| 409 | Slot changed during the request | `{"error": "slot was modified concurrently, retry"}` |

> **Concurrency:** if several users book the same slot at the same time, exactly one gets
> `200`; everyone else gets `409 slot is already booked`. In the UI, treat a `409` here as
> "someone else got it" — show a message and refresh the slot list.

### POST /slots/{id}/cancel

Cancel a booking, or release a hold. Only the user who booked or is holding the slot can do this.

**Request**
```json
{ "userId": 2 }
```

**Responses**

| Status | When | Body |
|---|---|---|
| 200 | Cancelled / released | [Slot](#slot) with `status: 0` and no `bookedBy` |
| 400 | Bad `id`, or `userId` missing / not a positive integer | `{"error": "invalid id"}`, or a [field-validation message](#error-reference) |
| 403 | Booked or held by a different user | `{"error": "slot is booked by another user"}` |
| 404 | No such slot | `{"error": "record not found"}` |
| 409 | Slot is free (including an expired hold) | `{"error": "slot is not booked"}` |
| 409 | Slot changed during the request | `{"error": "slot was modified concurrently, retry"}` |

### POST /slots/{id}/reschedule

Move the user's booking from slot `{id}` to another slot, in one step: the old slot is
freed and the new one booked, or nothing changes. Only the user who **booked** `{id}` can
reschedule it (a hold can't be rescheduled; cancel it and hold the other slot instead).
The target can be free, an expired hold, or a slot the same user is holding.
Recorded in history as one `rescheduled` entry.

**Request**
```json
{ "userId": 1, "toSlotId": 2 }
```

| Field | Type | Required | Notes |
|---|---|---|---|
| `userId` | integer | yes | Must be the user who booked slot `{id}` |
| `toSlotId` | integer | yes | The slot to move to; must differ from `{id}` |

**Responses**

| Status | When | Body |
|---|---|---|
| 200 | Rescheduled | The **new** [Slot](#slot), with `status: 100` and `bookedBy` set |
| 400 | Bad `id` or body | `{"error": "invalid id"}`, or a [field-validation message](#error-reference) |
| 400 | `toSlotId` equals `{id}` | `{"error": "cannot reschedule to the same slot"}` |
| 403 | Slot `{id}` is booked by a different user | `{"error": "slot is booked by another user"}` |
| 404 | Either slot doesn't exist | `{"error": "record not found"}` |
| 409 | Slot `{id}` isn't booked (free or only on hold) | `{"error": "slot is not booked"}` |
| 409 | Target slot is booked | `{"error": "slot is already booked"}` |
| 409 | Another user has an active hold on the target | `{"error": "slot is on hold"}` |
| 409 | A slot changed during the request | `{"error": "slot was modified concurrently, retry"}` |

> **Concurrency:** if two users reschedule into the same free slot at the same time, exactly
> one succeeds; the other gets `409 slot is already booked` and **keeps their original slot**.

---

## Error reference

| Status | `error` message | Meaning |
|---|---|---|
| 400 | `invalid id` | Path `id` isn't a positive integer |
| 400 | `invalid limit`, `invalid offset`, `limit must be between 1 and 200`, `offset must not be negative` | Bad history paging |
| 400 | `cannot reschedule to the same slot` | `toSlotId` equals the path `id` |
| 400 | `email is invalid`, `name is required` | User validation failed |
| 400 | `from must be before to`, `date must be in YYYY-MM-DD format`, `date, from and to are required` | Slot validation failed |
| 400 | `Key: '...' Error:Field validation for '...' failed on the 'required' tag` | A required JSON field is missing or has the wrong type |
| 403 | `slot is booked by another user` | Cancel or reschedule attempted by someone other than the booker or holder |
| 404 | `record not found` | User or slot doesn't exist |
| 409 | `record already exists` | Email already registered |
| 409 | `slot is already booked` | Holding, booking or rescheduling into a slot that's booked |
| 409 | `slot is on hold` | Slot has an active hold by another user (or you tried to re-hold your own) |
| 409 | `slot is not booked` | Cancelling or rescheduling a slot that's free |
| 409 | `slot was modified concurrently, retry` | Another request changed the slot first |
| 500 | `internal server error` | Unexpected server error (details are in the server log) |

## Quick test with curl

```sh
B=http://localhost:4500

curl -X POST $B/users -H 'Content-Type: application/json' \
  -d '{"name":"Asha","email":"asha@example.com"}'

curl -X POST $B/slots/bulk -H 'Content-Type: application/json' -d '{"slots":[
  {"date":"2026-10-10","from":"2026-10-10T10:00:00Z","to":"2026-10-10T11:00:00Z"},
  {"date":"2026-10-10","from":"2026-10-10T11:00:00Z","to":"2026-10-10T12:00:00Z"}]}'

curl -X POST $B/slots/1/hold   -H 'Content-Type: application/json' -d '{"userId":1}'
curl -X POST $B/slots/1/book   -H 'Content-Type: application/json' -d '{"userId":1}'
curl -X POST $B/slots/1/reschedule -H 'Content-Type: application/json' -d '{"userId":1,"toSlotId":2}'
curl -X POST $B/slots/2/cancel -H 'Content-Type: application/json' -d '{"userId":1}'
curl $B/slots
curl $B/users/1/history
```
