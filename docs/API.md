# Task Manager API Reference

A lightweight REST API for managing tasks, built with Go's `net/http`. Tasks are stored in a **PostgreSQL database** (Neon), so data persists across restarts.

- **Base URL:** `http://localhost:8080`
- **Default port:** `8080` (override with the `PORT` environment variable)
- **Database:** connection string from `Neon_db` (or `DATABASE_URL`); see the [README](../README.md#configuration)
- **Content-Type:** `application/json` for all request/response bodies (except the welcome and error responses, which are plain text)

**Tasks are private.** Every `/tasks` endpoint requires a session, and only ever
returns or modifies tasks belonging to the signed-in user.

---

## Authentication

A user signs in in one of two ways:

- **Email and password** — `POST /auth/register`, then `POST /auth/login`.
- **Google** — send the browser to `GET /auth/google`.

Either way the result is a **session**, which the client presents on subsequent
requests in one of two ways:

| Client | How it authenticates |
|--------|----------------------|
| Browser | The `session` cookie, set automatically on login. It is `HttpOnly` (page scripts cannot read it) and `SameSite=Lax` (other origins cannot drive the API with it). |
| Script, curl, mobile app | The `token` from the login response, sent as `Authorization: Bearer <token>`. |

The `Authorization` header wins when both are present. Sessions last **7 days**
from the moment they are issued.

```bash
# Sign in and keep the token
TOKEN=$(curl -s -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"ada@example.com","password":"correct-horse"}' \
  | grep -o '"token":"[^"]*"' | cut -d'"' -f4)

curl http://localhost:8080/tasks -H "Authorization: Bearer $TOKEN"
```

A request with no session, or with an expired or logged-out one, gets `401`.

> **Note:** Google sign-in is optional. If `GOOGLE_CLIENT_ID` and
> `GOOGLE_CLIENT_SECRET` are not configured, the two Google routes answer `501`
> and everything else works as normal.

---

## Data Model

### Task

| Field       | Type      | Description                                  |
|-------------|-----------|----------------------------------------------|
| `id`        | `integer` | Unique identifier, assigned by the database. |
| `title`     | `string`  | The task description.                         |
| `completed` | `boolean` | Whether the task is done.                     |

```json
{
  "id": 1,
  "title": "Write documentation",
  "completed": false
}
```

> **Note:** `id` is assigned by the database on creation. Any `id` supplied in a request body when creating a task is ignored.
>
> A task's owner is **not** part of this model. It is taken from the session, so
> a request body cannot create or move a task into someone else's account.

### User

| Field   | Type      | Description                                       |
|---------|-----------|---------------------------------------------------|
| `id`    | `integer` | Unique identifier, assigned by the database.      |
| `email` | `string`  | Lowercased. Unique across accounts.               |
| `name`  | `string`  | Display name. Empty unless supplied or from Google. |

```json
{
  "id": 1,
  "email": "ada@example.com",
  "name": "Ada Lovelace"
}
```

The password hash and Google account ID are stored but never returned.

---

## Endpoints

### 1. Welcome

```
GET /
```

Returns a plain-text greeting to confirm the API is running. Matches the root
path only, not every unrecognised path. Requires no session.

**Response** — `200 OK` (`text/plain`)

```
Welcome to the Task Manager API!
```

---

### 2. Register

```
POST /auth/register
```

Creates an account from an email and password, and signs it in — the response
carries a session, so no separate login call is needed.

**Request body**

| Field      | Type     | Required | Description                                     |
|------------|----------|----------|-------------------------------------------------|
| `email`    | `string` | Yes      | A bare address. Case and surrounding whitespace are normalized away. |
| `password` | `string` | Yes      | 8–72 bytes. The upper bound is bcrypt's own limit. |
| `name`     | `string` | No       | Display name.                                    |

```json
{
  "email": "ada@example.com",
  "password": "correct-horse",
  "name": "Ada Lovelace"
}
```

**Response** — `201 Created` (`application/json`)

```json
{
  "user": { "id": 1, "email": "ada@example.com", "name": "Ada Lovelace" },
  "token": "K1s9x2...",
  "expires_at": "2026-07-23T18:18:30Z"
}
```

Also sets the `session` cookie.

**Errors**

| Status | Body                                    | When                                        |
|--------|-----------------------------------------|---------------------------------------------|
| `400`  | `Invalid Data`                          | The request body is not valid JSON.         |
| `400`  | `Invalid Email Address`                 | The email is malformed, or carries a display name (`Ada <ada@example.com>`). |
| `400`  | `Password must be at least 8 characters` | The password is too short.                 |
| `400`  | `Password must be at most 72 bytes`     | The password is too long.                   |
| `409`  | `Email Already Registered`              | An account exists with that email.          |
| `500`  | `Could Not Create Account`              | The database insert failed.                 |

**Example**

```bash
curl -X POST http://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"ada@example.com","password":"correct-horse","name":"Ada Lovelace"}'
```

---

### 3. Log in

```
POST /auth/login
```

Exchanges an email and password for a new session. Each login issues a fresh
token; existing sessions keep working until they expire or are logged out.

**Request body**

```json
{
  "email": "ada@example.com",
  "password": "correct-horse"
}
```

**Response** — `200 OK` (`application/json`) — same shape as register.

**Errors**

| Status | Body                       | When                                         |
|--------|----------------------------|----------------------------------------------|
| `400`  | `Invalid Data`             | The request body is not valid JSON.          |
| `401`  | `Invalid Email Or Password` | The email is unknown, the password is wrong, or the account has no password because it was created through Google. |
| `500`  | `Could Not Sign In`        | The database query failed.                   |

> One message covers every failure on purpose: distinguishing them would turn
> this route into a way to test which addresses hold accounts.

**Example**

```bash
curl -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"ada@example.com","password":"correct-horse"}'
```

---

### 4. Log out

```
POST /auth/logout
```

Ends the session presented, and clears the cookie. Requires no session of its
own: logging out without one still returns `204`, because the outcome the caller
wants is the outcome either way.

**Response** — `204 No Content` (empty body)

**Errors**

| Status | Body                | When                        |
|--------|---------------------|-----------------------------|
| `500`  | `Could Not Sign Out` | The database delete failed. |

**Example**

```bash
curl -X POST http://localhost:8080/auth/logout -H "Authorization: Bearer $TOKEN"
```

---

### 5. Current user

```
GET /auth/me
```

Returns the signed-in user's account.

**Response** — `200 OK` (`application/json`)

```json
{ "id": 1, "email": "ada@example.com", "name": "Ada Lovelace" }
```

**Errors**

| Status | Body                        | When                                     |
|--------|-----------------------------|------------------------------------------|
| `401`  | `Authentication Required`   | No session was presented.                |
| `401`  | `Invalid Or Expired Session` | The session is unknown, expired, or logged out. |
| `401`  | `Account No Longer Exists`  | The account was deleted mid-session.     |
| `500`  | `Could Not Fetch Account`   | The database query failed.               |

**Example**

```bash
curl http://localhost:8080/auth/me -H "Authorization: Bearer $TOKEN"
```

---

### 6. Google sign-in

```
GET /auth/google
```

Starts Google sign-in. **Open this in a browser** — it responds `302` to
Google's consent screen and sets two short-lived cookies that the callback needs.
It is not a JSON endpoint and cannot be driven from curl.

**If you are already signed in**, this route *links* — it connects the Google
account to the one you hold, so you can use either from then on.

**If you are not signed in**, it signs you in. The account is matched in this
order:

1. An account already linked to that Google account.
2. An account holding the same email → **refused with `409`**. It is not adopted.
   Sign in with that account's password first, then visit this route again to
   connect Google.
3. Otherwise a new account, with no password, is created.

> **Why step 2 refuses rather than links.** Registration never proves an email
> belongs to whoever typed it. If a Google sign-in adopted any account sharing
> its address, an attacker could register your email before you ever did, wait
> for you to sign in with Google, and keep a password into the account you now
> think is yours. Google vouching for the address proves the person at the
> consent screen owns it — not that whoever created the local account did.
> Holding a live session for that account is the proof that closes the gap,
> which is what the signed-in path above requires.

**Response** — `302 Found` to `accounts.google.com`.

**Errors**

| Status | Body                              | When                                     |
|--------|-----------------------------------|------------------------------------------|
| `501`  | `Google Sign-In Is Not Configured` | No `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET`. |
| `500`  | `Could Not Start Google Sign-In`  | Random state could not be generated.     |

---

### 7. Google callback

```
GET /auth/google/callback
```

Where Google returns the user. **Google calls this, not you.** The URL must be
registered as an authorized redirect URI on the OAuth client and match
`GOOGLE_REDIRECT_URL` exactly.

On success it sets the `session` cookie and redirects to `POST_LOGIN_REDIRECT`
(default `/`). The token is deliberately not put in the URL, where it would leak
into browser history, server logs, and the next page's `Referer` header.

**Response** — `302 Found` to `POST_LOGIN_REDIRECT`.

**Errors**

| Status | Body                                            | When                                          |
|--------|-------------------------------------------------|-----------------------------------------------|
| `400`  | `Google Sign-In Expired, Please Try Again`      | The sign-in cookies are gone — more than 10 minutes passed, or the flow did not start here. |
| `400`  | `Google Sign-In Could Not Be Verified`          | The `state` does not match its cookie. A forged callback lands here. |
| `400`  | `Google Sign-In Returned No Code`               | No `code` parameter.                          |
| `401`  | `Google Sign-In Was Not Completed`              | The user pressed cancel.                      |
| `403`  | `Google Account Email Is Not Verified`          | Google does not vouch for the address, so it cannot be trusted to identify anyone. |
| `409`  | `Email Already Registered. Sign In With Your Password, Then Connect Google From Your Account` | A local account holds this email and has not been linked. See above. |
| `409`  | `This Account Is Already Connected To A Different Google Account` | You are signed in, and your account is already linked elsewhere. |
| `409`  | `Google Account Is Already Linked To Another User` | That Google account belongs to a different local account. |
| `501`  | `Google Sign-In Is Not Configured`              | No credentials configured.                    |
| `502`  | `Could Not Complete Google Sign-In`             | Google rejected the exchange or was unreachable. |

---

### 8. List all tasks

```
GET /tasks
```

Returns the signed-in user's tasks, ordered by `id`. If they have none, the
response is an empty array `[]`.

**Response** — `200 OK` (`application/json`)

```json
[
  { "id": 1, "title": "Write documentation", "completed": false },
  { "id": 2, "title": "Review pull request", "completed": true }
]
```

**Errors**

| Status | Body                    | When                       |
|--------|-------------------------|----------------------------|
| `401`  | `Authentication Required` | No session was presented. |
| `500`  | `Could Not Fetch Tasks` | The database query failed. |

**Example**

```bash
curl http://localhost:8080/tasks -H "Authorization: Bearer $TOKEN"
```

---

### 9. Create a task

```
POST /tasks
```

Creates a task owned by the signed-in user. The server assigns the `id`.

**Request body**

| Field       | Type      | Required | Description                 |
|-------------|-----------|----------|-----------------------------|
| `title`     | `string`  | No       | The task description.        |
| `completed` | `boolean` | No       | Initial completion state.    |

```json
{
  "title": "Buy groceries",
  "completed": false
}
```

**Response** — `200 OK` (`application/json`)

```json
{
  "id": 3,
  "title": "Buy groceries",
  "completed": false
}
```

**Errors**

| Status | Body                    | When                                |
|--------|-------------------------|-------------------------------------|
| `400`  | `Invalid Data`          | The request body is not valid JSON. |
| `401`  | `Authentication Required` | No session was presented.         |
| `500`  | `Could Not Create Task` | The database insert failed.         |

**Example**

```bash
curl -X POST http://localhost:8080/tasks \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"title":"Buy groceries","completed":false}'
```

---

### 10. Get a task by ID

```
GET /tasks/{id}
```

Returns one of the signed-in user's tasks.

**Path parameter**

| Name | Type      | Description        |
|------|-----------|--------------------|
| `id` | `integer` | The task's ID.     |

**Response** — `200 OK` (`application/json`)

```json
{
  "id": 1,
  "title": "Write documentation",
  "completed": false
}
```

**Errors**

| Status | Body                   | When                                |
|--------|------------------------|-------------------------------------|
| `400`  | `Invalid task ID`      | The `id` is missing or non-numeric. |
| `401`  | `Authentication Required` | No session was presented.        |
| `404`  | `Task Not Found`       | No such task **of yours**.          |
| `500`  | `Could Not Fetch Task` | The database query failed.          |

> Someone else's task is reported as `404`, not `403`: that a given ID exists at
> all is not something a stranger is entitled to learn.

**Example**

```bash
curl http://localhost:8080/tasks/1 -H "Authorization: Bearer $TOKEN"
```

---

### 11. Update a task

```
PUT /tasks/{id}
```

Replaces the `title` and `completed` fields of one of the signed-in user's tasks.

**Path parameter**

| Name | Type      | Description        |
|------|-----------|--------------------|
| `id` | `integer` | The task's ID.     |

**Request body**

```json
{
  "title": "Write documentation (updated)",
  "completed": true
}
```

**Response** — `200 OK` (`application/json`)

```json
{
  "id": 1,
  "title": "Write documentation (updated)",
  "completed": true
}
```

**Errors**

| Status | Body                    | When                                |
|--------|-------------------------|-------------------------------------|
| `400`  | `Invalid task ID`       | The `id` is missing or non-numeric. |
| `400`  | `Invalid Data`          | The request body is not valid JSON. |
| `401`  | `Authentication Required` | No session was presented.         |
| `404`  | `Task Not Found`        | No such task **of yours**.          |
| `500`  | `Could Not Update Task` | The database update failed.         |

**Example**

```bash
curl -X PUT http://localhost:8080/tasks/1 \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"title":"Write documentation (updated)","completed":true}'
```

---

### 12. Delete a task

```
DELETE /tasks/{id}
```

Removes one of the signed-in user's tasks.

**Path parameter**

| Name | Type      | Description        |
|------|-----------|--------------------|
| `id` | `integer` | The task's ID.     |

**Response** — `204 No Content` (empty body)

**Errors**

| Status | Body                    | When                                |
|--------|-------------------------|-------------------------------------|
| `400`  | `Invalid task ID`       | The `id` is missing or non-numeric. |
| `401`  | `Authentication Required` | No session was presented.         |
| `404`  | `Task Not Found`        | No such task **of yours**.          |
| `500`  | `Could Not Delete Task` | The database delete failed.         |

**Example**

```bash
curl -X DELETE http://localhost:8080/tasks/1 -H "Authorization: Bearer $TOKEN"
```

---

## HTTP Status Codes

| Code  | Meaning                                                          |
|-------|------------------------------------------------------------------|
| `200` | Success — resource returned.                                     |
| `201` | Success — account created.                                       |
| `204` | Success — resource deleted or session ended, no body returned.   |
| `302` | Redirect — to Google, or back after signing in.                  |
| `400` | Bad request — invalid ID, malformed JSON, or failed validation.  |
| `401` | Unauthenticated — no session, a dead one, or bad credentials.    |
| `403` | Forbidden — the Google account's email is not verified.          |
| `404` | Not found — no task of yours matches the requested ID.           |
| `405` | Method not allowed — unsupported HTTP method for the route.      |
| `409` | Conflict — the email or Google account is already registered.    |
| `500` | Server error — the database query failed.                        |
| `501` | Not implemented — Google sign-in is not configured.              |
| `502` | Bad gateway — Google rejected the exchange or was unreachable.   |

Error responses are returned as **plain text** (the message shown in the tables above), not JSON.

---

## Endpoint & Method Matrix

| Route                    | GET                 | POST        | PUT           | DELETE        | Session |
|--------------------------|---------------------|-------------|---------------|---------------|---------|
| `/`                      | Welcome             | 405         | 405           | 405           | No      |
| `/auth/register`         | 405                 | Register    | 405           | 405           | No      |
| `/auth/login`            | 405                 | Log in      | 405           | 405           | No      |
| `/auth/logout`           | 405                 | Log out     | 405           | 405           | Optional |
| `/auth/me`               | Current user        | 405         | 405           | 405           | **Yes** |
| `/auth/google`           | Start Google        | 405         | 405           | 405           | No      |
| `/auth/google/callback`  | Finish Google       | 405         | 405           | 405           | No      |
| `/tasks`                 | List tasks          | Create task | 405           | 405           | **Yes** |
| `/tasks/{id}`            | Get by ID           | 405         | Update task   | Delete task   | **Yes** |

Any method not listed for a route returns `405 Method Not Allowed`, along with
an `Allow` header naming the methods the route does accept.

Routes are matched exactly. A path that is not in the table above — including
`/tasks/1/` with a trailing slash — returns `404 page not found`.

---

## Logging

The server logs requests that result in errors via the `respondError` helper, using severity derived from the status code:

- `INFO` — status `< 400`
- `WARN` — status `400–499`
- `ERROR` — status `>= 500`

Log lines include the method, path, status, message, and (when present) the underlying error. Timestamps include date, time, and microseconds.

Credentials are never logged: the underlying error is recorded server-side only,
and passwords and session tokens are not part of it.
