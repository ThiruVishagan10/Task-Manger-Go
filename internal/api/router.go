// Package api exposes the task store over HTTP.
package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/ThiruVishagan10/Task-Manger-Go/internal/task"
)

// TaskStore is the persistence behaviour the API depends on. It is declared
// here, at the point of use, so that handlers can be exercised against a fake
// without a database.
type TaskStore interface {
	Create(ctx context.Context, t *task.Task) error
	List(ctx context.Context) ([]task.Task, error)
	Get(ctx context.Context, id int) (task.Task, error)
	Update(ctx context.Context, id int, t task.Task) (task.Task, error)
	Delete(ctx context.Context, id int) error
}

// Server holds the dependencies shared by every handler.
type Server struct {
	tasks TaskStore
}

// NewServer returns a Server backed by tasks.
func NewServer(tasks TaskStore) *Server {
	return &Server{tasks: tasks}
}

// Routes returns a handler serving every endpoint of the API.
//
// Patterns carry their method, so ServeMux answers unsupported methods with 405
// and an Allow header on its own.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", s.handleWelcome)

	mux.HandleFunc("GET /tasks", s.handleListTasks)
	mux.HandleFunc("POST /tasks", s.handleCreateTask)

	mux.HandleFunc("GET /tasks/{id}", s.handleGetTask)
	mux.HandleFunc("PUT /tasks/{id}", s.handleUpdateTask)
	mux.HandleFunc("DELETE /tasks/{id}", s.handleDeleteTask)

	return mux
}

// handleWelcome reports that the API is up.
func (s *Server) handleWelcome(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "Welcome to the Task Manager API!")
}
