# Task Manager API Reference

A lightweight REST API for managing tasks, built with Go's `net/http`. Tasks are stored in a **PostgreSQL database** (Neon), so data persists across restarts.

- **Base URL:** `http://localhost:8080`
- **Default port:** `8080` (override with the `PORT` environment variable)
- **Database:** connection string from `Neon_db` (or `DATABASE_URL`); see the [README](../README.md#configuration)
- **Content-Type:** `application/json` for all request/response bodies (except the welcome and error responses, which are plain text)

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

---

## Endpoints

### 1. Welcome

```
GET /
```

Returns a plain-text greeting to confirm the API is running. Matches the root
path only, not every unrecognised path.

**Response** — `200 OK` (`text/plain`)

```
Welcome to the Task Manager API!
```

---

### 2. List all tasks

```
GET /tasks
```

Returns every task currently stored.

**Response** — `200 OK` (`application/json`)

```json
[
  { "id": 1, "title": "Write documentation", "completed": false },
  { "id": 2, "title": "Review pull request", "completed": true }
]
```

Tasks are returned ordered by `id`. If no tasks exist, the response is an empty array `[]`.

**Errors**

| Status | Body                    | When                     |
|--------|-------------------------|--------------------------|
| `500`  | `Could Not Fetch Tasks` | The database query failed. |

**Example**

```bash
curl http://localhost:8080/tasks
```

---

### 3. Create a task

```
POST /tasks
```

Creates a new task. The server assigns the `id` automatically.

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
| `500`  | `Could Not Create Task` | The database insert failed.         |

**Example**

```bash
curl -X POST http://localhost:8080/tasks \
  -H "Content-Type: application/json" \
  -d '{"title":"Buy groceries","completed":false}'
```

---

### 4. Get a task by ID

```
GET /tasks/{id}
```

Returns a single task by its `id`.

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
| `404`  | `Task Not Found`       | No task exists with that `id`.      |
| `500`  | `Could Not Fetch Task` | The database query failed.          |

**Example**

```bash
curl http://localhost:8080/tasks/1
```

---

### 5. Update a task

```
PUT /tasks/{id}
```

Replaces the `title` and `completed` fields of an existing task.

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
| `404`  | `Task Not Found`        | No task exists with that `id`.      |
| `500`  | `Could Not Update Task` | The database update failed.         |

**Example**

```bash
curl -X PUT http://localhost:8080/tasks/1 \
  -H "Content-Type: application/json" \
  -d '{"title":"Write documentation (updated)","completed":true}'
```

---

### 6. Delete a task

```
DELETE /tasks/{id}
```

Removes a task by its `id`.

**Path parameter**

| Name | Type      | Description        |
|------|-----------|--------------------|
| `id` | `integer` | The task's ID.     |

**Response** — `204 No Content` (empty body)

**Errors**

| Status | Body                    | When                                |
|--------|-------------------------|-------------------------------------|
| `400`  | `Invalid task ID`       | The `id` is missing or non-numeric. |
| `404`  | `Task Not Found`        | No task exists with that `id`.      |
| `500`  | `Could Not Delete Task` | The database delete failed.         |

**Example**

```bash
curl -X DELETE http://localhost:8080/tasks/1
```

---

## HTTP Status Codes

| Code  | Meaning                                                        |
|-------|---------------------------------------------------------------|
| `200` | Success — resource returned.                                   |
| `204` | Success — resource deleted, no body returned.                  |
| `400` | Bad request — invalid ID or malformed JSON.                    |
| `404` | Not found — no task matches the requested ID.                  |
| `405` | Method not allowed — unsupported HTTP method for the route.    |
| `500` | Server error — the database query failed.                      |

Error responses are returned as **plain text** (the message shown in the tables above), not JSON.

---

## Endpoint & Method Matrix

| Route          | GET             | POST        | PUT           | DELETE        |
|----------------|-----------------|-------------|---------------|---------------|
| `/`            | Welcome         | 405         | 405           | 405           |
| `/tasks`       | List tasks      | Create task | 405           | 405           |
| `/tasks/{id}`  | Get by ID       | 405         | Update task   | Delete task   |

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
