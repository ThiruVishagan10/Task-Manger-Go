package task

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store persists tasks in PostgreSQL.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore returns a Store backed by pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Create inserts t and fills in the database-assigned ID.
func (s *Store) Create(ctx context.Context, t *Task) error {
	err := s.pool.QueryRow(ctx,
		`INSERT INTO tasks (title, completed) VALUES ($1, $2) RETURNING id`,
		t.Title, t.Completed,
	).Scan(&t.ID)
	if err != nil {
		return fmt.Errorf("insert task: %w", err)
	}

	return nil
}

// List returns every task, ordered by ID.
func (s *Store) List(ctx context.Context) ([]Task, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, title, completed FROM tasks ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("query tasks: %w", err)
	}
	defer rows.Close()

	// Non-nil so that an empty table encodes as [] rather than null.
	tasks := []Task{}

	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.Title, &t.Completed); err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		tasks = append(tasks, t)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tasks: %w", err)
	}

	return tasks, nil
}

// Get returns the task with the given ID, or ErrNotFound.
func (s *Store) Get(ctx context.Context, id int) (Task, error) {
	var t Task

	err := s.pool.QueryRow(ctx,
		`SELECT id, title, completed FROM tasks WHERE id = $1`, id,
	).Scan(&t.ID, &t.Title, &t.Completed)

	if errors.Is(err, pgx.ErrNoRows) {
		return Task{}, ErrNotFound
	}
	if err != nil {
		return Task{}, fmt.Errorf("query task %d: %w", id, err)
	}

	return t, nil
}

// Update replaces the title and completed fields of the task with the given ID
// and returns the stored result, or ErrNotFound.
func (s *Store) Update(ctx context.Context, id int, t Task) (Task, error) {
	var updated Task

	err := s.pool.QueryRow(ctx,
		`UPDATE tasks SET title = $1, completed = $2 WHERE id = $3
		 RETURNING id, title, completed`,
		t.Title, t.Completed, id,
	).Scan(&updated.ID, &updated.Title, &updated.Completed)

	if errors.Is(err, pgx.ErrNoRows) {
		return Task{}, ErrNotFound
	}
	if err != nil {
		return Task{}, fmt.Errorf("update task %d: %w", id, err)
	}

	return updated, nil
}

// Delete removes the task with the given ID, or returns ErrNotFound.
func (s *Store) Delete(ctx context.Context, id int) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM tasks WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete task %d: %w", id, err)
	}

	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}
