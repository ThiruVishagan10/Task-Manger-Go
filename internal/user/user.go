// Package user contains the account model and its PostgreSQL storage.
package user

import (
	"errors"
	"strings"
)

var (
	// ErrNotFound is returned by Store when no account matches the lookup.
	// Callers should match it with errors.Is rather than inspecting driver
	// errors, so that the storage backend stays an implementation detail.
	ErrNotFound = errors.New("user not found")

	// ErrEmailTaken is returned by Store when an account already exists with
	// the requested email.
	ErrEmailTaken = errors.New("email already registered")
)

// User is an account. ID is assigned by the database on creation.
//
// A user may hold either credential or both: PasswordHash is empty for accounts
// that only ever signed in with Google, and GoogleID is empty until a Google
// account is linked. Neither is ever serialised to a client — PasswordHash
// because it is a secret, GoogleID because clients have no use for it.
type User struct {
	ID           int    `json:"id"`
	Email        string `json:"email"`
	Name         string `json:"name"`
	PasswordHash string `json:"-"`
	GoogleID     string `json:"-"`
}

// NormalizeEmail puts an address into the form used for storage and lookup.
//
// Addresses arrive with inconsistent case and stray whitespace, and users do
// not expect "Ada@Example.com" and "ada@example.com" to be different accounts.
// Every read and write of the email column goes through this, which is what
// makes the UNIQUE constraint reject case-variant duplicates.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
