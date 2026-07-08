# Task Manager (Go)

A minimal RESTful Task Manager API written in Go using only the standard library (`net/http`). It provides basic CRUD operations over an in-memory list of tasks — no database or external dependencies required.

> ⚠️ **In-memory storage:** All tasks live in memory and are lost when the server stops.

---

## Features

- Create, read, update, and delete tasks (CRUD)
- Zero external dependencies — pure Go standard library
- Configurable port via a `.env` file or the `PORT` environment variable
- Structured request logging with severity levels (`INFO` / `WARN` / `ERROR`)
- JSON request and response bodies

---

## Requirements

- [Go](https://go.dev/dl/) 1.26.4 or later

---

## Getting Started

### 1. Clone the repository

```bash
git clone <repository-url>
cd Task-Manger-Go
```

### 2. Run the server

```bash
go run .
```

The server starts on port `8080` by default:

```
Server running on :8080
```

### 3. Build a binary (optional)

```bash
go build -o task-manager .
./task-manager        # on Windows: task-manager.exe
```

---

## Configuration

The port is resolved in the following order of precedence:

1. `PORT` environment variable
2. A `PORT` entry in a `.env` file in the project root
3. Default: `8080`

Example `.env` file:

```env
PORT=3000
```

The `.env` loader ignores blank lines and lines starting with `#`, and will not override variables that are already set in the environment.

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
├── main.go        # Entry point, HTTP routing, server startup
├── handlers.go    # Request handlers for task CRUD operations
├── task.go        # Task data model
├── config.go      # Configuration loading (.env + PORT)
├── logger.go      # Error responses and severity-based logging
└── go.mod         # Module definition
```

---

## Notes & Limitations

- Data is **not persisted** — restarting the server clears all tasks.
- No authentication or authorization.
- Error responses are returned as plain text, not JSON.
- Not safe for concurrent writes — the in-memory task slice is not guarded by a mutex, so it is intended for single-instance, low-concurrency use or local development.

---

## License

No license specified.
