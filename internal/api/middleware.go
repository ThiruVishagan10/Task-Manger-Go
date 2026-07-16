package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/ThiruVishagan10/Task-Manger-Go/internal/auth"
)

// sessionCookieName is the cookie a browser client is authenticated by.
const sessionCookieName = "session"

// bearerPrefix is the scheme a non-browser client presents its token under.
const bearerPrefix = "Bearer "

// contextKey is unexported so that no other package can write to this
// request's context slot: the only way an ID gets in is through requireAuth.
type contextKey struct{}

// userIDKey addresses the authenticated user's ID within a request context.
var userIDKey contextKey

// requireAuth rejects a request that carries no live session, and passes the
// authenticated user's ID down to next through the request context.
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, ok := sessionToken(r)
		if !ok {
			respondError(w, r, http.StatusUnauthorized, "Authentication Required", nil)
			return
		}

		id, err := s.sessions.UserID(r.Context(), token)
		if errors.Is(err, auth.ErrSessionInvalid) {
			// The client is holding a token that will never work again;
			// clearing it saves the browser from sending it on every request
			// from here on.
			s.clearSessionCookie(w)
			respondError(w, r, http.StatusUnauthorized, "Invalid Or Expired Session", nil)
			return
		}
		if err != nil {
			respondError(w, r, http.StatusInternalServerError, "Could Not Verify Session", err)
			return
		}

		next(w, r.WithContext(context.WithValue(r.Context(), userIDKey, id)))
	}
}

// userID returns the authenticated user's ID for a request served through
// requireAuth.
//
// It returns 0 for a request that was not, which is a real user ID for nobody:
// a handler wired up without requireAuth by mistake would find no tasks and
// create them for an owner that violates the foreign key, rather than operating
// on someone else's data.
func userID(r *http.Request) int {
	id, _ := r.Context().Value(userIDKey).(int)

	return id
}

// sessionToken pulls the session token off a request.
//
// Two ways in, because there are two kinds of client: a browser, which holds an
// HttpOnly cookie it cannot be tricked into reading out, and a script or curl
// session, which holds the token itself. The header wins when both are present,
// since an explicit Authorization header is a deliberate act and an ambient
// cookie is not.
func sessionToken(r *http.Request) (string, bool) {
	if header := r.Header.Get("Authorization"); header != "" {
		token, found := strings.CutPrefix(header, bearerPrefix)
		if found && token != "" {
			return token, true
		}

		return "", false
	}

	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return "", false
	}

	return cookie.Value, true
}

// setSessionCookie hands a session to a browser.
//
// HttpOnly keeps the token away from page scripts, so a cross-site scripting
// bug cannot read it out. SameSite=Lax keeps it off cross-site form posts,
// which is what stops another origin from driving this API as the signed-in
// user, while still allowing the top-level redirect back from Google to arrive
// authenticated.
func (s *Server) setSessionCookie(w http.ResponseWriter, session auth.Session) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    session.Token,
		Path:     "/",
		Expires:  session.ExpiresAt,
		HttpOnly: true,
		Secure:   s.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

// clearSessionCookie tells the browser to drop the session cookie. The
// attributes other than Expires and MaxAge must match those it was set with, or
// the browser will keep the original alongside it.
func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}
