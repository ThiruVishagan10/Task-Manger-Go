package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var db *pgxpool.Pool

const schema = `
CREATE TABLE IF NOT EXISTS tasks (
	id        SERIAL PRIMARY KEY,
	title     TEXT    NOT NULL DEFAULT '',
	completed BOOLEAN NOT NULL DEFAULT FALSE
);`

// initDB opens the connection pool, verifies the database is reachable, and
// makes sure the tasks table exists.
func initDB(ctx context.Context, url string) error {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return fmt.Errorf("parse database url: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return fmt.Errorf("connect to database: %w", err)
	}

	if _, err := pool.Exec(ctx, schema); err != nil {
		pool.Close()
		return fmt.Errorf("create tasks table: %w", err)
	}

	db = pool
	return nil
}
