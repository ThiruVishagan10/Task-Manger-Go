// Package auth issues and verifies the credentials a user signs in with:
// passwords, login sessions, and Google OAuth 2.0 identities.
package auth

import (
	"fmt"
	"sync"

	"golang.org/x/crypto/bcrypt"
)

const (
	// MinPasswordLength is the shortest password accepted at registration.
	MinPasswordLength = 8

	// MaxPasswordLength is the longest password accepted, in bytes.
	//
	// This is bcrypt's own limit, not a policy choice: it silently ignores
	// everything past the 72nd byte, so a longer password would only be
	// verified up to that point. Rejecting it is honest; truncating it is not.
	// The unit is bytes rather than characters because that is what bcrypt
	// counts — a multi-byte character costs more than one.
	MaxPasswordLength = 72
)

var (
	// ErrPasswordTooShort and ErrPasswordTooLong are returned by
	// ValidatePassword. Their messages are safe to show a user.
	ErrPasswordTooShort = fmt.Errorf("password must be at least %d characters", MinPasswordLength)
	ErrPasswordTooLong  = fmt.Errorf("password must be at most %d bytes", MaxPasswordLength)
)

// dummyHash is bcrypt over a value nobody knows, computed once on first use.
//
// Verifying a password against it costs the same as verifying a real one, which
// is what lets a login for an unregistered email take as long as one for a
// registered email. Without it, response time alone would tell an attacker
// which addresses hold accounts.
var dummyHash = sync.OnceValue(func() []byte {
	hash, err := bcrypt.GenerateFromPassword([]byte("password-for-an-account-that-does-not-exist"), bcrypt.DefaultCost)
	if err != nil {
		// Unreachable: the input is a fixed, valid-length literal.
		panic("auth: generate dummy hash: " + err.Error())
	}

	return hash
})

// ValidatePassword reports whether password is acceptable to HashPassword.
//
// It is separate from HashPassword so a handler can reject a bad password
// before paying for a hash, and so the reason can be shown to the user.
func ValidatePassword(password string) error {
	if len(password) < MinPasswordLength {
		return ErrPasswordTooShort
	}

	if len(password) > MaxPasswordLength {
		return ErrPasswordTooLong
	}

	return nil
}

// HashPassword returns a bcrypt hash of password, suitable for storage. The
// hash carries its own salt and cost, so nothing else needs to be kept.
func HashPassword(password string) (string, error) {
	if err := ValidatePassword(password); err != nil {
		return "", err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}

	return string(hash), nil
}

// VerifyPassword reports whether password matches hash.
//
// An empty hash means the account has no password — it was created through
// Google — and never matches. That case still runs a full bcrypt comparison, so
// that "no password set" and "wrong password" cannot be told apart by timing.
func VerifyPassword(hash, password string) bool {
	if hash == "" {
		VerifyNothing(password)
		return false
	}

	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// VerifyNothing spends the cost of a password check without having a password
// to check, and discards the result.
//
// Call it on the path where no account was found, so that login timing does not
// reveal whether an email is registered.
func VerifyNothing(password string) {
	_ = bcrypt.CompareHashAndPassword(dummyHash(), []byte(password))
}
