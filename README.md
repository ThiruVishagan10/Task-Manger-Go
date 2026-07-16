# Task Manager (Go)

A minimal RESTful Task Manager API written in Go with `net/http`. It provides basic CRUD operations over tasks stored in a **PostgreSQL database** ([Neon](https://neon.tech)). Users sign in with an email and password or with Google, and each user's tasks are private to them.

---

## Features

Everything below is implemented and working.

### Tasks

- Create, read, update, and delete tasks (CRUD) over a JSON REST API
- Every task is private to the user who created it: each statement filters by
  owner, so an ownership check cannot be forgotten at a call site
- A task belonging to someone else answers `404`, never `403` — that an ID
  exists at all is not something a stranger is entitled to learn
- The owner is taken from the session and never from the request body, so a
  client cannot create a task in somebody else's account
- Tasks list in a stable order (by ID), and an empty list encodes as `[]`
  rather than `null`

### Accounts and sign-in

- **Email and password** registration and login, hashed with bcrypt
- **Google sign-in** with the OAuth 2.0 authorization code flow and PKCE
- **Account linking** — sign in with your password, then visit `/auth/google`
  to connect a Google account to the account you already hold
- Google sign-in is **optional**: leave the credentials unset and the API runs
  on email and password alone, with the Google routes reporting `501`
- Addresses are normalized (trimmed and lowercased) on every read and write, so
  `Ada@Example.com` and `ada@example.com` are one account and not two
- Passwords are validated at 8–72 bytes before a hash is ever computed
- Registration rejects display-name forms like `Ada <ada@example.com>`, which
  would otherwise let one address register under two spellings
- Returning Google users are matched on Google's immutable subject ID rather
  than their email, so changing a Google address still lands on the same tasks

### Sessions

- Sessions live in the database, so a logout takes effect immediately
- Two ways to authenticate, for two kinds of client: a `session` cookie for
  browsers, and an `Authorization: Bearer` token for curl, scripts, and mobile
  clients. The header wins when both are present
- Cookies are `HttpOnly` (cross-site scripting cannot read the token out) and
  `SameSite=Lax` (another origin cannot drive the API as the signed-in user)
- Sessions expire 7 days after they are issued, enforced on every lookup rather
  than by a background job
- Expired sessions are swept hourly, so the table cannot grow without bound
- Only the SHA-256 of each token is stored, so a leaked database dump yields no
  usable sessions
- Every login issues a fresh token, so a token captured before a login cannot be
  used after it
- A dead or unknown token clears the client's cookie, rather than leaving the
  browser to resend it forever

### Hardening

- Login costs the same whether or not the email is registered — an unknown
  address still pays for a bcrypt comparison — and both failures return one
  message, so the route cannot be used to test which addresses hold accounts
- Google's `state` is checked against a cookie in constant time, so a forged
  callback cannot sign a browser into an attacker's account
- PKCE binds the authorization code to the browser that started the sign-in, so
  an intercepted code is useless on its own
- An address Google does not report as verified is refused
- A Google sign-in never adopts an existing password account on an email match
  alone, which closes an account pre-hijacking hole
- Error responses carry a message and never the underlying error, which is
  logged instead

### Storage and operations

- Persistent storage in PostgreSQL via [`pgx`](https://github.com/jackc/pgx)
  connection pooling
- Tables and indexes created automatically on startup, idempotently, from a
  schema embedded in the binary
- An existing pre-authentication `tasks` table is upgraded in place without
  losing rows
- Configurable database URL, port, OAuth credentials, and cookie policy via a
  `.env` file or environment variables, with real environment variables winning
- Structured request logging with severity levels (`INFO` / `WARN` / `ERROR`)
  derived from the response status
- Graceful shutdown: `Ctrl+C` drains in-flight requests, with a 10-second bound
- Fails fast at startup — a missing connection string or an unreachable database
  exits immediately rather than on the first request
- Unsupported methods answer `405` with an `Allow` header, handled by the router
- Calls out to Google are bounded by a 10-second timeout, so a sign-in cannot
  hang a request on an unresponsive dependency

---

## Requirements

- [Go](https://go.dev/dl/) 1.26.4 or later
- A PostgreSQL database (this project targets [Neon](https://neon.tech))
- Optional: a [Google OAuth 2.0 client](https://console.cloud.google.com/apis/credentials), for Google sign-in

---

## Getting Started

### 1. Clone the repository

```bash
git clone <repository-url>
cd Task-Manger-Go
```

### 2. Configure the database

Copy the example file and fill in your Neon connection string:

```bash
cp .env.example .env
```

```env
Neon_db='postgresql://<user>:<password>@<host>.neon.tech/<database>?sslmode=require&channel_binding=require'
```

`.env` is git-ignored — never commit real credentials.

### 3. Set up Google sign-in (optional)

Skip this to run with email and password only; the `/auth/google` routes will
answer `501` and nothing else is affected.

1. In the [Google Cloud console](https://console.cloud.google.com/apis/credentials),
   create an **OAuth 2.0 Client ID** of type **Web application**.
2. Under **Authorized redirect URIs**, add exactly:
   `http://localhost:8080/auth/google/callback`
3. Copy the client ID and secret into `.env`:

```env
GOOGLE_CLIENT_ID='<your-client-id>.apps.googleusercontent.com'
GOOGLE_CLIENT_SECRET='<your-client-secret>'
```

The redirect URI must match `GOOGLE_REDIRECT_URL` character for character —
Google rejects the sign-in before the user sees a consent screen otherwise.

### 4. Run the server

```bash
go run ./cmd/server
```

Run it from the project root: the `.env` file is read from the working
directory.

On startup the server connects to the database, applies the schema, reports
which sign-in methods came up, and listens on port `8080` by default:

```
Connected to database
Google sign-in enabled, redirecting to http://localhost:8080/auth/google/callback
[WARN] Session cookies are not marked Secure, so they will travel over plain HTTP. Set COOKIE_SECURE=true when serving over HTTPS.
Server running on :8080
```

That warning is expected during local development over `http://localhost`, and
is what you silence with `COOKIE_SECURE=true` in production.

If no connection string is configured, or the database is unreachable, the
server exits immediately with an error rather than starting.

Press `Ctrl+C` to stop; in-flight requests are given up to 10 seconds to finish
before the process exits.

### 5. Build a binary (optional)

```bash
go build -o task-manager ./cmd/server
./task-manager        # on Windows: task-manager.exe
```

---

## Configuration

| Variable               | Required | Default | Description                                      |
|------------------------|----------|---------|--------------------------------------------------|
| `Neon_db`              | Yes      | —       | PostgreSQL connection string. Falls back to `DATABASE_URL` if unset. |
| `PORT`                 | No       | `8080`  | Port the HTTP server listens on.                 |
| `GOOGLE_CLIENT_ID`     | No       | —       | OAuth client ID. Google sign-in is enabled only when this and the secret are both set. |
| `GOOGLE_CLIENT_SECRET` | No       | —       | OAuth client secret.                             |
| `GOOGLE_REDIRECT_URL`  | No       | `http://localhost:8080/auth/google/callback` | Must be registered verbatim on the OAuth client. |
| `POST_LOGIN_REDIRECT`  | No       | `/`     | Where a browser lands after Google sign-in. Point it at your frontend. |
| `COOKIE_SECURE`        | No       | `false` | Marks session cookies `Secure`. **Set to `true` in production.** |

Each value is resolved in this order of precedence:

1. The real environment variable
2. An entry in a `.env` file in the project root
3. The default shown above (where one exists)

Example `.env` file:

```env
PORT=3000
Neon_db='postgresql://user:password@host.neon.tech/neondb?sslmode=require&channel_binding=require'
GOOGLE_CLIENT_ID='<your-client-id>.apps.googleusercontent.com'
GOOGLE_CLIENT_SECRET='<your-client-secret>'
COOKIE_SECURE=false
```

`COOKIE_SECURE` defaults to `false` because a `Secure` cookie is never sent back
over plain HTTP: on `http://localhost` every login would appear to succeed and
then be instantly forgotten. In production, over HTTPS, it must be `true`.

The `.env` loader ignores blank lines and lines starting with `#`, strips surrounding quotes, and will not override variables that are already set in the environment.

---

## Database Schema

Created automatically on startup:

```sql
CREATE TABLE IF NOT EXISTS users (
    id            SERIAL PRIMARY KEY,
    email         TEXT        NOT NULL UNIQUE,
    name          TEXT        NOT NULL DEFAULT '',
    password_hash TEXT,                      -- NULL for Google-only accounts
    google_id     TEXT        UNIQUE,        -- NULL until a Google account is linked
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS sessions (
    token_hash BYTEA       PRIMARY KEY,      -- SHA-256 of the token, never the token
    user_id    INTEGER     NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS tasks (
    id        SERIAL PRIMARY KEY,
    title     TEXT    NOT NULL DEFAULT '',
    completed BOOLEAN NOT NULL DEFAULT FALSE,
    user_id   INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE
);

-- Supporting indexes, created on startup alongside the tables.
CREATE INDEX IF NOT EXISTS sessions_user_id_idx    ON sessions (user_id);
CREATE INDEX IF NOT EXISTS sessions_expires_at_idx ON sessions (expires_at);
CREATE INDEX IF NOT EXISTS tasks_user_id_idx       ON tasks (user_id);
```

`id` is assigned by Postgres, so IDs remain unique across restarts. Deleting a
user takes their tasks and sessions with it.

The indexes cover the three lookups that happen constantly: every task query
filters by `user_id`, the cascade from a deleted user finds that user's
sessions, and the hourly sweep scans by `expires_at`.

### Upgrading a database that predates authentication

`tasks.user_id` is added by `ALTER TABLE ... ADD COLUMN IF NOT EXISTS` on
startup. Tasks created before authentication existed have no owner, and nothing
in the schema can guess who that owner should have been, so:

- **An empty or new `tasks` table** gets `user_id NOT NULL` immediately, and
  ownership is enforced by the database from then on.
- **A table holding pre-auth tasks** keeps the column nullable, and those rows
  become invisible — every query filters by `user_id`. They are *not* deleted.
  Assign them an owner (`UPDATE tasks SET user_id = <id> WHERE user_id IS NULL`)
  or delete them; the constraint tightens by itself on the next startup once
  none are left.

---

## API Overview

| Method   | Endpoint                 | Description                     | Session  |
|----------|--------------------------|---------------------------------|----------|
| `GET`    | `/`                      | Welcome message                 | No       |
| `POST`   | `/auth/register`         | Create an account and sign in   | No       |
| `POST`   | `/auth/login`            | Sign in with email and password | No       |
| `POST`   | `/auth/logout`           | End the current session         | Optional |
| `GET`    | `/auth/me`               | The signed-in user's account    | **Yes**  |
| `GET`    | `/auth/google`           | Start Google sign-in (browser)  | No       |
| `GET`    | `/auth/google/callback`  | Finish Google sign-in           | No       |
| `GET`    | `/tasks`                 | List your tasks                 | **Yes**  |
| `POST`   | `/tasks`                 | Create a task                   | **Yes**  |
| `GET`    | `/tasks/{id}`            | Get one of your tasks           | **Yes**  |
| `PUT`    | `/tasks/{id}`            | Update one of your tasks        | **Yes**  |
| `DELETE` | `/tasks/{id}`            | Delete one of your tasks        | **Yes**  |

📖 See [docs/API.md](docs/API.md) for the full API reference, including request/response examples and error codes.

### Quick example

```bash
# Create an account (also signs you in)
curl -X POST http://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"ada@example.com","password":"correct-horse","name":"Ada"}'

# Sign in and keep the token
TOKEN=$(curl -s -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"ada@example.com","password":"correct-horse"}' \
  | grep -o '"token":"[^"]*"' | cut -d'"' -f4)

# Create a task
curl -X POST http://localhost:8080/tasks \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"title":"Learn Go","completed":false}'

# List your tasks
curl http://localhost:8080/tasks -H "Authorization: Bearer $TOKEN"
```

For Google sign-in, open `http://localhost:8080/auth/google` in a browser. A
browser client can skip the token entirely — the `session` cookie is set on
login and sent automatically.

---

## Project Structure

```
Task-Manger-Go/
├── cmd/
│   └── server/
│       └── main.go            # Entry point: wiring, startup, graceful shutdown
├── internal/
│   ├── api/                   # HTTP layer
│   │   ├── router.go          # Routes, Server type, store interfaces
│   │   ├── middleware.go      # requireAuth, session cookies
│   │   ├── auth_handler.go    # Register, login, logout, current user
│   │   ├── google_handler.go  # Google OAuth start and callback
│   │   ├── task_handler.go    # Request handlers for task CRUD
│   │   └── response.go        # JSON/error responses and severity logging
│   ├── auth/                  # Credentials
│   │   ├── password.go        # bcrypt hashing and verification
│   │   ├── session.go         # Session tokens and their storage
│   │   └── google.go          # Google OAuth 2.0 client
│   ├── config/
│   │   └── config.go          # Configuration loading (.env + environment)
│   ├── database/
│   │   ├── database.go        # Connection pool setup
│   │   ├── migrate.go         # Schema application on startup
│   │   └── schema.sql         # Embedded table definitions
│   ├── task/
│   │   ├── task.go            # Task model and domain errors
│   │   └── store.go           # PostgreSQL persistence
│   └── user/
│       ├── user.go            # User model and domain errors
│       └── store.go           # PostgreSQL persistence
├── docs/
│   └── API.md                 # Full API reference
├── .env.example               # Template for .env
├── go.mod                     # Module definition
└── go.sum                     # Dependency checksums
```

**Why this shape:**

- `cmd/server` holds the entry point, so the module can grow more binaries (a
  migration tool, a seeder) without disturbing the server.
- `internal/` is enforced by the Go toolchain: nothing outside this module can
  import these packages, so they stay free to change.
- Dependencies point one way — `api` → `auth`/`task`/`user` → `database`. The
  domain packages never import `api`, and `api` never imports pgx.
- `api` declares the interfaces it needs at the point of use, so handlers can be
  tested against a fake with no database and no round trip to Google.
- The stores translate driver errors into domain errors (`task.ErrNotFound`,
  `user.ErrEmailTaken`), so swapping PostgreSQL for another backend would not
  touch the HTTP layer.
- `config` stays a leaf package: `main` reads the credentials out of it and
  constructs the Google client, rather than `config` importing `auth`.

---

## How Authentication Works

- **Passwords** are hashed with bcrypt at its default cost. Passwords are capped
  at 72 bytes because that is bcrypt's own limit — it ignores everything past
  it, so a longer password would only be checked up to that point. Rejecting one
  is honest; silently truncating it is not.
- **Sessions** are 32 random bytes and last 7 days from the moment they are
  issued. Only their SHA-256 is stored, so a leaked database dump yields no
  usable sessions. Lookups reject expired rows in the `WHERE` clause, so a
  logout or an expiry takes effect immediately rather than whenever the hourly
  sweep next runs. The sweep is housekeeping — it keeps the table from growing
  without bound — and not the thing that enforces expiry.
- **Cookies** are `HttpOnly` (a cross-site scripting bug cannot read the token
  out) and `SameSite=Lax` (another origin cannot drive the API as the signed-in
  user, while the redirect back from Google still arrives authenticated).
- **Google sign-in** uses the authorization code flow with PKCE and a
  random `state` checked against a cookie, so a forged callback cannot log a
  browser into an attacker's account. The profile is read from Google's
  `userinfo` endpoint over TLS rather than decoded from the `id_token`, which
  keeps a JWT library, a key cache, and a class of signature-verification bugs
  out of the project.
- **Account linking** requires proving you hold the account: sign in with its
  password, then visit `/auth/google`. A Google sign-in never *adopts* an
  existing account just because the emails match. That would be an account
  pre-hijacking hole — registration does not prove an address belongs to whoever
  typed it, so an attacker could register your email before you did, wait for
  you to arrive through Google, and keep a working password into the account you
  believe is yours. Linking in the other direction is guarded too: an address
  Google does not report as verified is refused, so nobody can claim an email at
  an identity provider to reach the matching local account.
- **Enumeration** is guarded against: a login for an unknown email runs a
  throwaway bcrypt comparison so it costs the same as a real one, and both
  failures return the same message. Another user's task is `404`, never `403`.

---

## Notes & Limitations

- Tasks are persisted in PostgreSQL and survive restarts, and are private to the
  user who created them.
- **No rate limiting.** Nothing slows down repeated login attempts, so a weak
  password can be attacked offline-fast. This is the most significant gap; put a
  limiter in front of `/auth/login` and `/auth/register` before exposing this to
  the internet.
- **No password reset, and no email verification** for password accounts — there
  is no mail delivery in this project. An address registered with a password is
  not proved to belong to whoever registered it. (Addresses arriving via Google
  *are* verified, by Google.)
- Sessions cannot be listed or revoked individually by the user; logout ends only
  the session it is called with. Deleting a user's row ends all of theirs.
- Error responses are returned as plain text, not JSON.
- `PUT /tasks/{id}` is a full replace — omitting a field resets it to its zero value rather than leaving it unchanged.
- The schema is applied on startup and is additive only (`CREATE TABLE IF NOT EXISTS`, `ADD COLUMN IF NOT EXISTS`); there is no migration tooling for changes that need to drop or rewrite data.
- Routes are exact: `/tasks/1/` (trailing slash) and unknown paths return `404`.
- Because a Google sign-in will not adopt an existing account, someone who
  registers your email before you do can keep you from signing in with Google
  under that address — you get a `409` rather than their account. That is the
  deliberate trade: a lockout is recoverable, a silent takeover is not. Email
  verification at registration would remove the trade; it needs mail delivery,
  which this project does not have.
- Test coverage is limited to `internal/api/google_handler_test.go`, which
  exercises the account-resolution and CSRF logic of the Google flow against
  fakes — the highest-risk logic in the auth code. The rest is verified by hand.
  The store interfaces in `internal/api` exist so the rest can be tested the same
  way, without a database or a real Google client.

---

## License

No license specified.
