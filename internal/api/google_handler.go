package api

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"time"

	"github.com/ThiruVishagan10/Task-Manger-Go/internal/auth"
	"github.com/ThiruVishagan10/Task-Manger-Go/internal/user"
)

// The cookies that carry a sign-in across the round trip to Google. They are
// scoped to the callback path, so they travel with nothing else.
const (
	oauthStateCookie    = "oauth_state"
	oauthVerifierCookie = "oauth_verifier"
	oauthCookiePath     = "/auth/google"
)

// oauthCookieTTL is how long a sign-in may sit half-finished at Google's
// consent screen before the browser forgets it was ever started.
const oauthCookieTTL = 10 * time.Minute

// handleGoogleStart begins Google sign-in by redirecting to Google's consent
// screen.
func (s *Server) handleGoogleStart(w http.ResponseWriter, r *http.Request) {
	if !s.googleConfigured(w, r) {
		return
	}

	state, err := auth.NewState()
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "Could Not Start Google Sign-In", err)
		return
	}

	verifier, err := auth.NewPKCEVerifier()
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "Could Not Start Google Sign-In", err)
		return
	}

	s.setOAuthCookie(w, oauthStateCookie, state)
	s.setOAuthCookie(w, oauthVerifierCookie, verifier)

	http.Redirect(w, r, s.google.AuthCodeURL(state, auth.PKCEChallenge(verifier)), http.StatusFound)
}

// handleGoogleCallback completes Google sign-in and starts a session.
func (s *Server) handleGoogleCallback(w http.ResponseWriter, r *http.Request) {
	if !s.googleConfigured(w, r) {
		return
	}

	query := r.URL.Query()

	state, stateErr := r.Cookie(oauthStateCookie)
	verifier, verifierErr := r.Cookie(oauthVerifierCookie)

	// This sign-in attempt is over however it turns out, so retire its cookies
	// now rather than on the way out: a header set after the response has begun
	// is a header the client never sees, and a state cookie that survives its
	// callback is one that can be replayed against another.
	s.clearOAuthCookies(w)

	// The user pressed cancel, or Google refused the request outright.
	if reason := query.Get("error"); reason != "" {
		respondError(w, r, http.StatusUnauthorized, "Google Sign-In Was Not Completed", errors.New(reason))
		return
	}

	if stateErr != nil || verifierErr != nil {
		respondError(w, r, http.StatusBadRequest, "Google Sign-In Expired, Please Try Again", errors.Join(stateErr, verifierErr))
		return
	}

	// Constant-time, so that the comparison cannot be walked character by
	// character with a stopwatch.
	if subtle.ConstantTimeCompare([]byte(state.Value), []byte(query.Get("state"))) != 1 {
		respondError(w, r, http.StatusBadRequest, "Google Sign-In Could Not Be Verified", errors.New("state mismatch"))
		return
	}

	code := query.Get("code")
	if code == "" {
		respondError(w, r, http.StatusBadRequest, "Google Sign-In Returned No Code", nil)
		return
	}

	identity, err := s.google.Exchange(r.Context(), code, verifier.Value)
	if err != nil {
		respondError(w, r, http.StatusBadGateway, "Could Not Complete Google Sign-In", err)
		return
	}

	// An address Google does not vouch for proves nothing about who is signing
	// in, and accepting one would let a Workspace account claim any email.
	if !identity.EmailVerified {
		respondError(w, r, http.StatusForbidden, "Google Account Email Is Not Verified", nil)
		return
	}

	u, err := s.userForGoogleIdentity(r, identity)
	if err != nil {
		status, message := googleSignInFailure(err)
		respondError(w, r, status, message, err)
		return
	}

	session, err := s.sessions.Create(r.Context(), u.ID)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "Could Not Start Session", err)
		return
	}

	s.setSessionCookie(w, session)

	// A browser, not a script, is on the other end of this redirect — it
	// followed one to get here — so the session goes back as a cookie and the
	// token itself is never put in the URL, where it would leak into history,
	// logs, and the next page's Referer.
	http.Redirect(w, r, s.postLoginRedirect, http.StatusFound)
}

var (
	// errEmailBelongsToPasswordAccount is returned when a Google sign-in lands
	// on an email that an existing local account already holds.
	errEmailBelongsToPasswordAccount = errors.New("email already belongs to a local account")

	// errAlreadyLinked is returned when the signed-in account is already
	// connected to a different Google account.
	errAlreadyLinked = errors.New("account is already linked to a different google account")
)

// userForGoogleIdentity resolves a Google identity to a local account.
//
// The route does double duty. A caller who is already signed in is connecting a
// Google account to the one they hold; a caller who is not is signing in, and
// may end up with a new account.
func (s *Server) userForGoogleIdentity(r *http.Request, identity auth.GoogleIdentity) (user.User, error) {
	ctx := r.Context()

	if currentID, ok := s.signedInUserID(r); ok {
		return s.linkGoogleAccount(ctx, currentID, identity)
	}

	// The account has signed in with Google before. Matching on the Google
	// subject rather than the email means a user who has since changed their
	// Google address still lands on their own tasks.
	u, err := s.users.GetByGoogleID(ctx, identity.Sub)
	if err == nil {
		return u, nil
	}
	if !errors.Is(err, user.ErrNotFound) {
		return user.User{}, err
	}

	// A local account already holds this address, and this Google account has
	// never been seen. Adopting it would be a hijack waiting to happen.
	//
	// Google vouches for the address, but that only proves the person at the
	// consent screen owns it — it says nothing about who created the local row.
	// Registration never proves an address belongs to whoever typed it, so that
	// row may have been registered by someone who merely knew this address and
	// is waiting for its real owner to arrive through Google. Linking here would
	// hand that squatter an account its rightful owner believes is theirs,
	// unlocked by a password only the squatter knows.
	//
	// So the address is not enough: the caller must prove they hold the existing
	// account by signing in with its password and coming back, which is the
	// branch above.
	if _, err := s.users.GetByEmail(ctx, identity.Email); err == nil {
		return user.User{}, errEmailBelongsToPasswordAccount
	} else if !errors.Is(err, user.ErrNotFound) {
		return user.User{}, err
	}

	// Nobody holds this address. A new account, with no password, whose email is
	// trustworthy precisely because Google verified it and registration cannot.
	u = user.User{Email: identity.Email, Name: identity.Name, GoogleID: identity.Sub}
	if err := s.users.Create(ctx, &u); err != nil {
		return user.User{}, err
	}

	return u, nil
}

// linkGoogleAccount connects identity to the account the caller is signed in as.
//
// This is the only path that may attach a Google account to an account holding a
// password, and it is safe because holding a live session for it is proof of
// exactly what an email match is not: that the caller really is this user.
func (s *Server) linkGoogleAccount(ctx context.Context, currentID int, identity auth.GoogleIdentity) (user.User, error) {
	u, err := s.users.GetByID(ctx, currentID)
	if err != nil {
		return user.User{}, err
	}

	// Already connected — signing in again with the same Google account is not
	// an error, it is a no-op.
	if u.GoogleID == identity.Sub {
		return u, nil
	}

	if u.GoogleID != "" {
		return user.User{}, errAlreadyLinked
	}

	if err := s.users.LinkGoogleID(ctx, u.ID, identity.Sub); err != nil {
		return user.User{}, err
	}
	u.GoogleID = identity.Sub

	return u, nil
}

// signedInUserID returns the caller's user ID if the request carries a live
// session.
//
// A missing or dead session is not an error here: it means the caller is
// signing in rather than linking. A lookup failure is treated the same way,
// since the paths that follow either succeed on their own terms or surface
// their own error.
func (s *Server) signedInUserID(r *http.Request) (int, bool) {
	token, ok := sessionToken(r)
	if !ok {
		return 0, false
	}

	id, err := s.sessions.UserID(r.Context(), token)
	if err != nil {
		return 0, false
	}

	return id, true
}

// googleSignInFailure maps an account resolution failure to a response.
func googleSignInFailure(err error) (int, string) {
	switch {
	case errors.Is(err, errEmailBelongsToPasswordAccount):
		return http.StatusConflict, "Email Already Registered. Sign In With Your Password, Then Connect Google From Your Account"
	case errors.Is(err, errAlreadyLinked):
		return http.StatusConflict, "This Account Is Already Connected To A Different Google Account"
	case errors.Is(err, user.ErrGoogleAccountTaken):
		return http.StatusConflict, "Google Account Is Already Linked To Another User"
	case errors.Is(err, user.ErrEmailTaken):
		// Two callbacks for a brand-new address raced, and the other one
		// created the account a moment ago. Retrying signs in normally.
		return http.StatusConflict, "Account Was Just Created, Please Sign In Again"
	default:
		return http.StatusInternalServerError, "Could Not Complete Google Sign-In"
	}
}

// googleConfigured reports whether Google sign-in is available, answering the
// request itself when it is not.
//
// The routes stay mounted either way: an operator who has not set the
// credentials gets told exactly that, rather than a 404 that reads like a
// missing feature.
func (s *Server) googleConfigured(w http.ResponseWriter, r *http.Request) bool {
	if s.google == nil {
		respondError(w, r, http.StatusNotImplemented, "Google Sign-In Is Not Configured", nil)
		return false
	}

	return true
}

// setOAuthCookie stores one leg of an in-flight sign-in.
//
// SameSite=Lax is required rather than merely chosen: the callback arrives as a
// top-level navigation from accounts.google.com, and under SameSite=Strict the
// browser would withhold these cookies and every sign-in would fail its state
// check.
func (s *Server) setOAuthCookie(w http.ResponseWriter, name, value string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     oauthCookiePath,
		MaxAge:   int(oauthCookieTTL.Seconds()),
		HttpOnly: true,
		Secure:   s.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

// clearOAuthCookies retires both legs of a finished sign-in.
func (s *Server) clearOAuthCookies(w http.ResponseWriter) {
	for _, name := range []string{oauthStateCookie, oauthVerifierCookie} {
		http.SetCookie(w, &http.Cookie{
			Name:     name,
			Value:    "",
			Path:     oauthCookiePath,
			Expires:  time.Unix(0, 0),
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   s.secureCookies,
			SameSite: http.SameSiteLaxMode,
		})
	}
}
