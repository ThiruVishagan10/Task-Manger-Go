package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

// parseTaskID pulls the {id} out of /tasks/{id}, responding with 400 itself if
// it is missing or non-numeric.
func parseTaskID(w http.ResponseWriter, r *http.Request) (int, bool) {
	idStr := strings.Trim(strings.TrimPrefix(r.URL.Path, "/tasks/"), "/")

	if idStr == "" {
		respondError(w, r, http.StatusBadRequest, "Invalid task ID", nil)
		return 0, false
	}

	id, err := strconv.Atoi(idStr)
	if err != nil {
		respondError(w, r, http.StatusBadRequest, "Invalid task ID", err)
		return 0, false
	}

	return id, true
}

func createTask(w http.ResponseWriter, r *http.Request) {
	var task Task

	err := json.NewDecoder(r.Body).Decode(&task)

	if err != nil {
		respondError(w, r, http.StatusBadRequest, "Invalid Data", err)
		return
	}

	err = db.QueryRow(r.Context(),
		`INSERT INTO tasks (title, completed) VALUES ($1, $2) RETURNING id`,
		task.Title, task.Completed,
	).Scan(&task.ID)

	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "Could Not Create Task", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	json.NewEncoder(w).Encode(task)
}

func getTasksByID(w http.ResponseWriter, r *http.Request) {
	id, ok := parseTaskID(w, r)
	if !ok {
		return
	}

	var task Task

	err := db.QueryRow(r.Context(),
		`SELECT id, title, completed FROM tasks WHERE id = $1`, id,
	).Scan(&task.ID, &task.Title, &task.Completed)

	if errors.Is(err, pgx.ErrNoRows) {
		respondError(w, r, http.StatusNotFound, "Task Not Found", nil)
		return
	}

	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "Could Not Fetch Task", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(task)
}

func getTasks(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(r.Context(), `SELECT id, title, completed FROM tasks ORDER BY id`)

	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "Could Not Fetch Tasks", err)
		return
	}
	defer rows.Close()

	tasks := []Task{}

	for rows.Next() {
		var task Task

		if err := rows.Scan(&task.ID, &task.Title, &task.Completed); err != nil {
			respondError(w, r, http.StatusInternalServerError, "Could Not Fetch Tasks", err)
			return
		}

		tasks = append(tasks, task)
	}

	if err := rows.Err(); err != nil {
		respondError(w, r, http.StatusInternalServerError, "Could Not Fetch Tasks", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tasks)
}

func updateTasks(w http.ResponseWriter, r *http.Request) {
	id, ok := parseTaskID(w, r)
	if !ok {
		return
	}

	var updateTasks Task

	err := json.NewDecoder(r.Body).Decode(&updateTasks)

	if err != nil {
		respondError(w, r, http.StatusBadRequest, "Invalid Data", err)
		return
	}

	var task Task

	err = db.QueryRow(r.Context(),
		`UPDATE tasks SET title = $1, completed = $2 WHERE id = $3
		 RETURNING id, title, completed`,
		updateTasks.Title, updateTasks.Completed, id,
	).Scan(&task.ID, &task.Title, &task.Completed)

	if errors.Is(err, pgx.ErrNoRows) {
		respondError(w, r, http.StatusNotFound, "Task Not Found", nil)
		return
	}

	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "Could Not Update Task", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(task)
}

func deleteTask(w http.ResponseWriter, r *http.Request) {
	id, ok := parseTaskID(w, r)
	if !ok {
		return
	}

	tag, err := db.Exec(r.Context(), `DELETE FROM tasks WHERE id = $1`, id)

	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "Could Not Delete Task", err)
		return
	}

	if tag.RowsAffected() == 0 {
		respondError(w, r, http.StatusNotFound, "Task Not Found", nil)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
