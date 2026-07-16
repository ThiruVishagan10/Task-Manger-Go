package database

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// schema is embedded at build time so the binary is self-contained and does not
// need schema.sql to be shipped alongside it.
//
//go:embed schema.sql
var schema string

// Migrate applies the schema. It is idempotent and runs on every startup.
//
// This is deliberately not a versioned migration system: it can only create
// what is missing, never alter or drop what already exists. Changing an
// existing column will need a real migration tool.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, schema); err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}

	return nil
}
