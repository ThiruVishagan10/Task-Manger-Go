package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/ThiruVishagan10/Task-Manger-Go/internal/task"
)

// handleCreateTask stores a new task and returns it with its assigned ID.
func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	var t task.Task

	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		respondError(w, r, http.StatusBadRequest, "Invalid Data", err)
		return
	}

	if err := s.tasks.Create(r.Context(), &t); err != nil {
		respondError(w, r, http.StatusInternalServerError, "Could Not Create Task", err)
		return
	}

	writeJSON(w, http.StatusOK, t)
}

// handleListTasks returns every stored task.
func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	tasks, err := s.tasks.List(r.Context())
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "Could Not Fetch Tasks", err)
		return
	}

	writeJSON(w, http.StatusOK, tasks)
}

// handleGetTask returns a single task by ID.
func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	id, ok := taskID(w, r)
	if !ok {
		return
	}

	t, err := s.tasks.Get(r.Context(), id)
	if errors.Is(err, task.ErrNotFound) {
		respondError(w, r, http.StatusNotFound, "Task Not Found", nil)
		return
	}
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "Could Not Fetch Task", err)
		return
	}

	writeJSON(w, http.StatusOK, t)
}

// handleUpdateTask replaces a task's title and completed fields.
func (s *Server) handleUpdateTask(w http.ResponseWriter, r *http.Request) {
	id, ok := taskID(w, r)
	if !ok {
		return
	}

	var t task.Task

	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		respondError(w, r, http.StatusBadRequest, "Invalid Data", err)
		return
	}

	updated, err := s.tasks.Update(r.Context(), id, t)
	if errors.Is(err, task.ErrNotFound) {
		respondError(w, r, http.StatusNotFound, "Task Not Found", nil)
		return
	}
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "Could Not Update Task", err)
		return
	}

	writeJSON(w, http.StatusOK, updated)
}

// handleDeleteTask removes a task by ID.
func (s *Server) handleDeleteTask(w http.ResponseWriter, r *http.Request) {
	id, ok := taskID(w, r)
	if !ok {
		return
	}

	err := s.tasks.Delete(r.Context(), id)
	if errors.Is(err, task.ErrNotFound) {
		respondError(w, r, http.StatusNotFound, "Task Not Found", nil)
		return
	}
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "Could Not Delete Task", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// taskID reads the {id} path segment, responding with 400 itself if it is not a
// number.
func taskID(w http.ResponseWriter, r *http.Request) (int, bool) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondError(w, r, http.StatusBadRequest, "Invalid task ID", err)
		return 0, false
	}

	return id, true
}
