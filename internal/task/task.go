// Package task contains the task domain model and its PostgreSQL storage.
package task

import "errors"

// ErrNotFound is returned by Store when no task exists with the requested ID.
// Callers should match it with errors.Is rather than inspecting driver errors,
// so that the storage backend stays an implementation detail.
var ErrNotFound = errors.New("task not found")

// Task is a single to-do item. ID is assigned by the database on creation.
type Task struct {
	ID        int    `json:"id"`
	Title     string `json:"title"`
	Completed bool   `json:"completed"`
}
