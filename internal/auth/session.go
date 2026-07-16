package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SessionTTL is how long a session stays valid after it is issued.
const SessionTTL = 7 * 24 * time.Hour

// tokenBytes is the entropy in a session token. 32 bytes puts guessing a valid
// token out of reach, and there is no reason to be thriftier.
const tokenBytes = 32

// ErrSessionInvalid is returned when a token matches no live session — it was
// never issued, it has expired, or it has been logged out. The cases are
// deliberately indistinguishable to the caller: a client can do nothing
// different about them, and telling them apart would leak whether a guessed
// token once existed.
var ErrSessionInvalid = errors.New("session invalid or expired")

// Session is a login session as handed to a client.
//
// Token is the only copy of the secret that ever exists in this form: the store
// keeps a hash, so a session cannot be reconstructed from the database.
type Session struct {
	Token     string
	ExpiresAt time.Time
}

// SessionStore persists sessions in PostgreSQL.
type SessionStore struct {
	pool *pgxpool.Pool
}

// NewSessionStore returns a SessionStore backed by pool.
func NewSessionStore(pool *pgxpool.Pool) *SessionStore {
	return &SessionStore{pool: pool}
}

// Create issues a new session for userID.
//
// Every login gets a fresh token rather than reusing one, so a token captured
// before a login cannot be used after it.
func (s *SessionStore) Create(ctx context.Context, userID int) (Session, error) {
	token, err := newToken()
	if err != nil {
		return Session{}, err
	}

	expiresAt := time.Now().Add(SessionTTL)

	_, err = s.pool.Exec(ctx,
		`INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ($1, $2, $3)`,
		hashToken(token), userID, expiresAt,
	)
	if err != nil {
		return Session{}, fmt.Errorf("insert session: %w", err)
	}

	return Session{Token: token, ExpiresAt: expiresAt}, nil
}

// UserID returns the user the token belongs to, or ErrSessionInvalid.
//
// Expiry is enforced here, in the WHERE clause, rather than by the sweep in
// DeleteExpired: a session must stop working the moment it expires, whether or
// not the row has been collected yet.
func (s *SessionStore) UserID(ctx context.Context, token string) (int, error) {
	var userID int

	err := s.pool.QueryRow(ctx,
		`SELECT user_id FROM sessions WHERE token_hash = $1 AND expires_at > NOW()`,
		hashToken(token),
	).Scan(&userID)

	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrSessionInvalid
	}
	if err != nil {
		return 0, fmt.Errorf("query session: %w", err)
	}

	return userID, nil
}

// Delete ends a session. Logging out an already-dead session is not an error:
// the caller wanted it gone, and it is.
func (s *SessionStore) Delete(ctx context.Context, token string) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1`, hashToken(token)); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}

	return nil
}

// DeleteExpired removes sessions that have timed out and reports how many were
// collected. Expired rows are already refused by UserID, so this is housekeeping
// to stop the table growing without bound, not a security boundary.
func (s *SessionStore) DeleteExpired(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE expires_at <= NOW()`)
	if err != nil {
		return 0, fmt.Errorf("delete expired sessions: %w", err)
	}

	return tag.RowsAffected(), nil
}

// newToken returns a URL-safe random session token.
func newToken() (string, error) {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(b), nil
}

// hashToken reduces a token to what the database is allowed to know.
//
// A plain SHA-256 is right here where it would be wrong for a password: tokens
// are 32 random bytes of our own making, so there is no guessable input for an
// offline attack to work through, and lookups happen on every authenticated
// request where bcrypt's cost would be felt.
func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))

	return sum[:]
}
