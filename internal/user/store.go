package user

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// uniqueViolation is the SQLSTATE Postgres reports for a broken UNIQUE
// constraint.
const uniqueViolation = "23505"

// ErrGoogleAccountTaken is returned when a Google account is already linked to
// a different user.
var ErrGoogleAccountTaken = errors.New("google account already linked to another user")

// Store persists users in PostgreSQL.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore returns a Store backed by pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// columns lists the user fields every query selects, in scan order.
const columns = `id, email, name, password_hash, google_id`

// Create inserts u and fills in the database-assigned ID. The email is
// normalized first, so callers cannot introduce case-variant duplicates.
//
// It returns ErrEmailTaken or ErrGoogleAccountTaken when the account collides
// with an existing one. That check belongs to the database rather than a prior
// SELECT: two concurrent registrations of the same address would both pass the
// SELECT and only the UNIQUE constraint would stop the second.
func (s *Store) Create(ctx context.Context, u *User) error {
	u.Email = NormalizeEmail(u.Email)

	err := s.pool.QueryRow(ctx,
		`INSERT INTO users (email, name, password_hash, google_id)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		u.Email, u.Name, nullIfEmpty(u.PasswordHash), nullIfEmpty(u.GoogleID),
	).Scan(&u.ID)
	if err != nil {
		if dup := duplicateError(err); dup != nil {
			return dup
		}
		return fmt.Errorf("insert user: %w", err)
	}

	return nil
}

// GetByID returns the user with the given ID, or ErrNotFound.
func (s *Store) GetByID(ctx context.Context, id int) (User, error) {
	return s.queryOne(ctx, `SELECT `+columns+` FROM users WHERE id = $1`, id)
}

// GetByEmail returns the user with the given email, or ErrNotFound.
func (s *Store) GetByEmail(ctx context.Context, email string) (User, error) {
	return s.queryOne(ctx, `SELECT `+columns+` FROM users WHERE email = $1`, NormalizeEmail(email))
}

// GetByGoogleID returns the user linked to the given Google account ID, or
// ErrNotFound.
func (s *Store) GetByGoogleID(ctx context.Context, googleID string) (User, error) {
	return s.queryOne(ctx, `SELECT `+columns+` FROM users WHERE google_id = $1`, googleID)
}

// LinkGoogleID attaches a Google account to an existing user, so that someone
// who registered with a password can later sign in with Google.
//
// It returns ErrGoogleAccountTaken if that Google account already belongs to
// someone else, and ErrNotFound if the user does not exist.
func (s *Store) LinkGoogleID(ctx context.Context, id int, googleID string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE users SET google_id = $1 WHERE id = $2`, googleID, id)
	if err != nil {
		if dup := duplicateError(err); dup != nil {
			return dup
		}
		return fmt.Errorf("link google account to user %d: %w", id, err)
	}

	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// queryOne runs a single-row lookup and maps a missing row to ErrNotFound.
func (s *Store) queryOne(ctx context.Context, sql string, args ...any) (User, error) {
	var (
		u                      User
		passwordHash, googleID *string
	)

	err := s.pool.QueryRow(ctx, sql, args...).Scan(&u.ID, &u.Email, &u.Name, &passwordHash, &googleID)

	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("query user: %w", err)
	}

	u.PasswordHash = derefOrEmpty(passwordHash)
	u.GoogleID = derefOrEmpty(googleID)

	return u, nil
}

// duplicateError translates a UNIQUE violation into the domain error for the
// column that collided, or returns nil if err is not a UNIQUE violation.
func duplicateError(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != uniqueViolation {
		return nil
	}

	if strings.Contains(pgErr.ConstraintName, "google_id") {
		return ErrGoogleAccountTaken
	}

	return ErrEmailTaken
}

// nullIfEmpty maps an empty string to a SQL NULL, keeping "absent credential"
// distinct from "empty credential" in the database.
func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}

	return &s
}

// derefOrEmpty maps a SQL NULL back to an empty string.
func derefOrEmpty(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}
