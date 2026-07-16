package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/ThiruVishagan10/Task-Manger-Go/internal/auth"
	"github.com/ThiruVishagan10/Task-Manger-Go/internal/user"
)

// invalidCredentials is returned for both an unknown email and a wrong
// password. One message for both, because saying which was wrong would turn the
// login route into a way to test whether an address holds an account.
const invalidCredentials = "Invalid Email Or Password"

// credentials is the body of a register or login request. Name is only read at
// registration.
type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

// authResponse is what a successful register or login returns.
//
// The token is in the body as well as in the cookie, so that clients without a
// cookie jar — curl, scripts, a mobile app — can authenticate by sending it
// back as a Bearer header. Browser clients should ignore it and let the cookie
// do the work, since a token read by page scripts is a token XSS can steal.
type authResponse struct {
	User      user.User `json:"user"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// handleRegister creates an account from an email and password and signs it in.
func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var creds credentials

	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		respondError(w, r, http.StatusBadRequest, "Invalid Data", err)
		return
	}

	email, err := parseEmail(creds.Email)
	if err != nil {
		respondError(w, r, http.StatusBadRequest, "Invalid Email Address", err)
		return
	}

	// Checked before hashing: no reason to spend a bcrypt round rejecting a
	// password we already know is too short.
	if err := auth.ValidatePassword(creds.Password); err != nil {
		respondError(w, r, http.StatusBadRequest, capitalize(err.Error()), nil)
		return
	}

	hash, err := auth.HashPassword(creds.Password)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "Could Not Create Account", err)
		return
	}

	u := user.User{Email: email, Name: creds.Name, PasswordHash: hash}

	err = s.users.Create(r.Context(), &u)
	if errors.Is(err, user.ErrEmailTaken) {
		respondError(w, r, http.StatusConflict, "Email Already Registered", nil)
		return
	}
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "Could Not Create Account", err)
		return
	}

	s.startSession(w, r, u, http.StatusCreated)
}

// handleLogin exchanges an email and password for a session.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var creds credentials

	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		respondError(w, r, http.StatusBadRequest, "Invalid Data", err)
		return
	}

	u, err := s.users.GetByEmail(r.Context(), creds.Email)
	if errors.Is(err, user.ErrNotFound) {
		// Spend the time a real password check would have cost, so that an
		// unregistered email is not given away by a fast rejection.
		auth.VerifyNothing(creds.Password)
		respondError(w, r, http.StatusUnauthorized, invalidCredentials, nil)
		return
	}
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "Could Not Sign In", err)
		return
	}

	if !auth.VerifyPassword(u.PasswordHash, creds.Password) {
		respondError(w, r, http.StatusUnauthorized, invalidCredentials, nil)
		return
	}

	s.startSession(w, r, u, http.StatusOK)
}

// handleLogout ends the caller's session.
//
// It does not require authentication: a client asking to be logged out gets its
// cookie cleared whether or not the token it presented was still good, because
// the outcome it wants — no session — is the outcome either way.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if token, ok := sessionToken(r); ok {
		if err := s.sessions.Delete(r.Context(), token); err != nil {
			respondError(w, r, http.StatusInternalServerError, "Could Not Sign Out", err)
			return
		}
	}

	s.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

// handleMe returns the signed-in user's account.
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	u, err := s.users.GetByID(r.Context(), userID(r))
	if errors.Is(err, user.ErrNotFound) {
		// The session outlived the account it belongs to: the row was deleted
		// between the lookup in requireAuth and this one.
		s.clearSessionCookie(w)
		respondError(w, r, http.StatusUnauthorized, "Account No Longer Exists", nil)
		return
	}
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "Could Not Fetch Account", err)
		return
	}

	writeJSON(w, http.StatusOK, u)
}

// startSession issues a session for u, sets the cookie, and writes the account
// and token back with the given status.
func (s *Server) startSession(w http.ResponseWriter, r *http.Request, u user.User, status int) {
	session, err := s.sessions.Create(r.Context(), u.ID)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "Could Not Start Session", err)
		return
	}

	s.setSessionCookie(w, session)
	writeJSON(w, status, authResponse{User: u, Token: session.Token, ExpiresAt: session.ExpiresAt})
}

// parseEmail validates an address and returns it in normalized form.
//
// mail.ParseAddress also accepts display-name forms like `Ada <ada@example.com>`,
// which would let two spellings of one address register twice; requiring the
// parsed address to survive normalization unchanged rejects them.
func parseEmail(email string) (string, error) {
	normalized := user.NormalizeEmail(email)

	addr, err := mail.ParseAddress(normalized)
	if err != nil {
		return "", err
	}

	if user.NormalizeEmail(addr.Address) != normalized {
		return "", errors.New("address must be a bare email, without a display name")
	}

	return normalized, nil
}

// capitalize upper-cases the first letter of an error message, so a validation
// reason reads like the other response messages.
func capitalize(s string) string {
	if s == "" {
		return s
	}

	return strings.ToUpper(s[:1]) + s[1:]
}
