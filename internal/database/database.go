// Package database manages the PostgreSQL connection pool and schema.
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// connectTimeout bounds the initial reachability check, so a bad host or
// firewall surfaces as a startup error instead of hanging.
const connectTimeout = 15 * time.Second

// Connect opens a connection pool for url and verifies the database is
// reachable. The caller owns the returned pool and must Close it.
func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect to database: %w", err)
	}

	return pool, nil
}
