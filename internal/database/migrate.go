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
// This is deliberately not a versioned migration system. Every statement in
// schema.sql must be safe to re-run and must never drop or rewrite existing
// data — additive changes only. Anything destructive, or any change that has to
// happen in a particular order relative to a deploy, needs a real migration
// tool.
//
// The statements are sent as one simple-protocol query, which is what allows a
// multi-statement script here: pgx only takes that path when Exec is called
// with no arguments.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, schema); err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}

	return nil
}
