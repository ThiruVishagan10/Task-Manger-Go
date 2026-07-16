# Task Manager (Go)

A minimal RESTful Task Manager API written in Go with `net/http`. It provides basic CRUD operations over tasks stored in a **PostgreSQL database** ([Neon](https://neon.tech)).

---

## Features

- Create, read, update, and delete tasks (CRUD)
- Persistent storage in PostgreSQL via [`pgx`](https://github.com/jackc/pgx) connection pooling
- Table schema created automatically on startup
- Configurable database URL and port via a `.env` file or environment variables
- Structured request logging with severity levels (`INFO` / `WARN` / `ERROR`)
- JSON request and response bodies

---

## Requirements

- [Go](https://go.dev/dl/) 1.26.4 or later
- A PostgreSQL database (this project targets [Neon](https://neon.tech))

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

### 3. Run the server

```bash
go run ./cmd/server
```

Run it from the project root: the `.env` file is read from the working
directory.

On startup the server connects to the database, applies the schema if the
`tasks` table does not exist, and listens on port `8080` by default:

```
Connected to database
Server running on :8080
```

If no connection string is configured, or the database is unreachable, the
server exits immediately with an error rather than starting.

Press `Ctrl+C` to stop; in-flight requests are given up to 10 seconds to finish
before the process exits.

### 4. Build a binary (optional)

```bash
go build -o task-manager ./cmd/server
./task-manager        # on Windows: task-manager.exe
```

---

## Configuration

| Variable    | Required | Default | Description                                      |
|-------------|----------|---------|--------------------------------------------------|
| `Neon_db`   | Yes      | —       | PostgreSQL connection string. Falls back to `DATABASE_URL` if unset. |
| `PORT`      | No       | `8080`  | Port the HTTP server listens on.                 |

Each value is resolved in this order of precedence:

1. The real environment variable
2. An entry in a `.env` file in the project root
3. The default shown above (where one exists)

Example `.env` file:

```env
PORT=3000
Neon_db='postgresql://user:password@host.neon.tech/neondb?sslmode=require&channel_binding=require'
```

The `.env` loader ignores blank lines and lines starting with `#`, strips surrounding quotes, and will not override variables that are already set in the environment.

---

## Database Schema

Created automatically on startup:

```sql
CREATE TABLE IF NOT EXISTS tasks (
    id        SERIAL PRIMARY KEY,
    title     TEXT    NOT NULL DEFAULT '',
    completed BOOLEAN NOT NULL DEFAULT FALSE
);
```

`id` is assigned by Postgres, so IDs remain unique across restarts.

---

## API Overview

| Method   | Endpoint        | Description               |
|----------|-----------------|---------------------------|
| `GET`    | `/`             | Welcome message           |
| `GET`    | `/tasks`        | List all tasks            |
| `POST`   | `/tasks`        | Create a new task         |
| `GET`    | `/tasks/{id}`   | Get a task by ID          |
| `PUT`    | `/tasks/{id}`   | Update a task by ID       |
| `DELETE` | `/tasks/{id}`   | Delete a task by ID       |

📖 See [docs/API.md](docs/API.md) for the full API reference, including request/response examples and error codes.

### Quick example

```bash
# Create a task
curl -X POST http://localhost:8080/tasks \
  -H "Content-Type: application/json" \
  -d '{"title":"Learn Go","completed":false}'

# List tasks
curl http://localhost:8080/tasks
```

---

## Project Structure

```
Task-Manger-Go/
├── cmd/
│   └── server/
│       └── main.go          # Entry point: wiring, startup, graceful shutdown
├── internal/
│   ├── api/                 # HTTP layer
│   │   ├── router.go        # Routes, Server type, TaskStore interface
│   │   ├── task_handler.go  # Request handlers for task CRUD
│   │   └── response.go      # JSON/error responses and severity logging
│   ├── config/
│   │   └── config.go        # Configuration loading (.env + PORT + Neon_db)
│   ├── database/
│   │   ├── database.go      # Connection pool setup
│   │   ├── migrate.go       # Schema application on startup
│   │   └── schema.sql       # Embedded table definitions
│   └── task/
│       ├── task.go          # Task model and domain errors
│       └── store.go         # PostgreSQL persistence
├── docs/
│   └── API.md               # Full API reference
├── .env.example             # Template for .env
├── go.mod                   # Module definition
└── go.sum                   # Dependency checksums
```

**Why this shape:**

- `cmd/server` holds the entry point, so the module can grow more binaries (a
  migration tool, a seeder) without disturbing the server.
- `internal/` is enforced by the Go toolchain: nothing outside this module can
  import these packages, so they stay free to change.
- Dependencies point one way — `api` → `task` → `database`. The `task` package
  never imports `api`, and `api` never imports pgx.
- `api` declares the `TaskStore` interface it needs at the point of use, so
  handlers can be tested against a fake with no database.
- The store translates driver errors into domain errors (`task.ErrNotFound`),
  so swapping PostgreSQL for another backend would not touch the HTTP layer.

---

## Notes & Limitations

- Tasks are persisted in PostgreSQL and survive restarts.
- No authentication or authorization.
- Error responses are returned as plain text, not JSON.
- `PUT /tasks/{id}` is a full replace — omitting a field resets it to its zero value rather than leaving it unchanged.
- The schema is created on startup with `CREATE TABLE IF NOT EXISTS`; there is no migration tooling for future schema changes.
- Routes are exact: `/tasks/1/` (trailing slash) and unknown paths return `404`.
- There is no test suite yet. The `TaskStore` interface in `internal/api` exists so handlers can be tested against a fake without a database.

---

## License

No license specified.
