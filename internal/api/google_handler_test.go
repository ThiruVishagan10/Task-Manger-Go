package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/ThiruVishagan10/Task-Manger-Go/internal/auth"
	"github.com/ThiruVishagan10/Task-Manger-Go/internal/user"
)

// The fakes below stand in for the stores and for Google, so these tests need
// neither a database nor a network. That is the whole reason the interfaces are
// declared in this package rather than taken from the store packages.

type fakeUserStore struct {
	mu     sync.Mutex
	users  map[int]user.User
	nextID int
}

func newFakeUserStore() *fakeUserStore {
	return &fakeUserStore{users: map[int]user.User{}, nextID: 1}
}

func (f *fakeUserStore) Create(_ context.Context, u *user.User) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	u.Email = user.NormalizeEmail(u.Email)

	for _, existing := range f.users {
		if existing.Email == u.Email {
			return user.ErrEmailTaken
		}
		if u.GoogleID != "" && existing.GoogleID == u.GoogleID {
			return user.ErrGoogleAccountTaken
		}
	}

	u.ID = f.nextID
	f.nextID++
	f.users[u.ID] = *u

	return nil
}

func (f *fakeUserStore) GetByID(_ context.Context, id int) (user.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	u, ok := f.users[id]
	if !ok {
		return user.User{}, user.ErrNotFound
	}

	return u, nil
}

func (f *fakeUserStore) GetByEmail(_ context.Context, email string) (user.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, u := range f.users {
		if u.Email == user.NormalizeEmail(email) {
			return u, nil
		}
	}

	return user.User{}, user.ErrNotFound
}

func (f *fakeUserStore) GetByGoogleID(_ context.Context, googleID string) (user.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, u := range f.users {
		if u.GoogleID != "" && u.GoogleID == googleID {
			return u, nil
		}
	}

	return user.User{}, user.ErrNotFound
}

func (f *fakeUserStore) LinkGoogleID(_ context.Context, id int, googleID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, u := range f.users {
		if u.GoogleID == googleID && u.ID != id {
			return user.ErrGoogleAccountTaken
		}
	}

	u, ok := f.users[id]
	if !ok {
		return user.ErrNotFound
	}

	u.GoogleID = googleID
	f.users[id] = u

	return nil
}

type fakeSessionStore struct {
	mu      sync.Mutex
	byToken map[string]int
	issued  int
}

func newFakeSessionStore() *fakeSessionStore {
	return &fakeSessionStore{byToken: map[string]int{}}
}

func (f *fakeSessionStore) Create(_ context.Context, userID int) (auth.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.issued++
	token := fmt.Sprintf("token-%d-for-user-%d", f.issued, userID)
	f.byToken[token] = userID

	return auth.Session{Token: token}, nil
}

func (f *fakeSessionStore) UserID(_ context.Context, token string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	id, ok := f.byToken[token]
	if !ok {
		return 0, auth.ErrSessionInvalid
	}

	return id, nil
}

func (f *fakeSessionStore) Delete(_ context.Context, token string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	delete(f.byToken, token)

	return nil
}

type fakeGoogle struct{ identity auth.GoogleIdentity }

func (f *fakeGoogle) AuthCodeURL(string, string) string { return "https://accounts.google.com/fake" }

func (f *fakeGoogle) Exchange(context.Context, string, string) (auth.GoogleIdentity, error) {
	return f.identity, nil
}

// testServer returns a Server wired to fakes, plus the fakes worth inspecting.
func testServer(identity auth.GoogleIdentity) (http.Handler, *fakeUserStore, *fakeSessionStore) {
	users := newFakeUserStore()
	sessions := newFakeSessionStore()

	srv := NewServer(Deps{
		Tasks:             nil, // unused: these tests never touch a task route
		Users:             users,
		Sessions:          sessions,
		Google:            &fakeGoogle{identity: identity},
		PostLoginRedirect: "/",
	})

	return srv.Routes(), users, sessions
}

// register creates an account and returns its session token.
func register(t *testing.T, h http.Handler, email, password string) string {
	t.Helper()

	body := fmt.Sprintf(`{"email":%q,"password":%q}`, email, password)
	req := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("register %s: status = %d, want %d", email, w.Code, http.StatusCreated)
	}

	for _, c := range w.Result().Cookies() {
		if c.Name == sessionCookieName {
			return c.Value
		}
	}

	t.Fatalf("register %s: no session cookie", email)

	return ""
}

// googleCallback drives a Google callback whose state cookie matches, i.e. one
// that has passed CSRF validation. sessionToken, when non-empty, signs the
// caller in first — which is what turns a sign-in into a link request.
func googleCallback(h http.Handler, sessionToken string) *httptest.ResponseRecorder {
	const state = "matching-state"

	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=any-code&state="+state, nil)
	req.AddCookie(&http.Cookie{Name: oauthStateCookie, Value: state})
	req.AddCookie(&http.Cookie{Name: oauthVerifierCookie, Value: "any-verifier"})

	if sessionToken != "" {
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionToken})
	}

	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	return w
}

// TestGoogleCallbackRefusesToAdoptAnUnverifiedEmailAccount is a regression test
// for an account pre-hijacking hole.
//
// Registration does not prove an address belongs to whoever typed it. So if a
// Google sign-in adopted any local account sharing its email, an attacker could
// register the victim's address ahead of time, wait for the victim to sign in
// with Google, and be handed a live password into the account the victim now
// believes is theirs.
func TestGoogleCallbackRefusesToAdoptAnUnverifiedEmailAccount(t *testing.T) {
	const victimEmail = "victim@corp.com"

	h, users, _ := testServer(auth.GoogleIdentity{
		Sub:           "google-sub-of-the-real-victim",
		Email:         victimEmail,
		EmailVerified: true,
		Name:          "Victim",
	})

	// The attacker squats on the victim's address before the victim arrives.
	register(t, h, victimEmail, "attacker-knows-this")

	// The victim signs in with Google, holding the same address for real.
	w := googleCallback(h, "")

	if w.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d — the Google identity must not be adopted into an unverified account", w.Code, http.StatusConflict)
	}

	// The decisive check: the squatted account must not have absorbed the
	// victim's Google identity. If it had, the attacker's password would now
	// open the victim's account.
	squatted, err := users.GetByEmail(context.Background(), victimEmail)
	if err != nil {
		t.Fatalf("GetByEmail: %v", err)
	}

	if squatted.GoogleID != "" {
		t.Errorf("attacker's account was linked to Google id %q; the victim's identity was hijacked", squatted.GoogleID)
	}

	if _, err := users.GetByGoogleID(context.Background(), "google-sub-of-the-real-victim"); !errors.Is(err, user.ErrNotFound) {
		t.Errorf("a Google link was created; want none")
	}
}

// TestGoogleCallbackLinksWhenSignedIn covers the safe way to link: holding a
// live session for the account is proof of what an email match is not.
func TestGoogleCallbackLinksWhenSignedIn(t *testing.T) {
	const email = "ada@example.com"

	h, users, _ := testServer(auth.GoogleIdentity{
		Sub:           "ada-google-sub",
		Email:         email,
		EmailVerified: true,
		Name:          "Ada",
	})

	token := register(t, h, email, "correct-horse")

	w := googleCallback(h, token)

	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusFound, w.Body)
	}

	linked, err := users.GetByEmail(context.Background(), email)
	if err != nil {
		t.Fatalf("GetByEmail: %v", err)
	}

	if linked.GoogleID != "ada-google-sub" {
		t.Errorf("GoogleID = %q, want %q — signing in and linking should connect the accounts", linked.GoogleID, "ada-google-sub")
	}

	if linked.PasswordHash == "" {
		t.Error("password was dropped by linking; the user should keep both ways in")
	}
}

// TestGoogleCallbackCreatesAccountForNewEmail covers a first-time Google user,
// whose address nobody holds.
func TestGoogleCallbackCreatesAccountForNewEmail(t *testing.T) {
	h, users, _ := testServer(auth.GoogleIdentity{
		Sub:           "newcomer-sub",
		Email:         "newcomer@example.com",
		EmailVerified: true,
		Name:          "Newcomer",
	})

	w := googleCallback(h, "")

	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusFound, w.Body)
	}

	created, err := users.GetByGoogleID(context.Background(), "newcomer-sub")
	if err != nil {
		t.Fatalf("GetByGoogleID: %v", err)
	}

	if created.Email != "newcomer@example.com" {
		t.Errorf("Email = %q, want %q", created.Email, "newcomer@example.com")
	}

	// A Google-only account has no password, and must not be reachable by
	// guessing an empty one.
	if created.PasswordHash != "" {
		t.Errorf("PasswordHash = %q, want empty for a Google-only account", created.PasswordHash)
	}
}

// TestGoogleCallbackSignsInReturningUser checks that a second visit lands on the
// same account rather than making another.
func TestGoogleCallbackSignsInReturningUser(t *testing.T) {
	h, users, _ := testServer(auth.GoogleIdentity{
		Sub:           "returning-sub",
		Email:         "returning@example.com",
		EmailVerified: true,
	})

	if w := googleCallback(h, ""); w.Code != http.StatusFound {
		t.Fatalf("first sign-in: status = %d, want %d", w.Code, http.StatusFound)
	}
	if w := googleCallback(h, ""); w.Code != http.StatusFound {
		t.Fatalf("second sign-in: status = %d, want %d", w.Code, http.StatusFound)
	}

	users.mu.Lock()
	count := len(users.users)
	users.mu.Unlock()

	if count != 1 {
		t.Errorf("account count = %d, want 1 — a returning user must not get a second account", count)
	}
}

// TestGoogleCallbackRejectsUnverifiedGoogleEmail covers the other direction: an
// address Google does not vouch for proves nothing about who is signing in.
func TestGoogleCallbackRejectsUnverifiedGoogleEmail(t *testing.T) {
	h, users, _ := testServer(auth.GoogleIdentity{
		Sub:           "unverified-sub",
		Email:         "someone-elses@corp.com",
		EmailVerified: false,
	})

	w := googleCallback(h, "")

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}

	if _, err := users.GetByGoogleID(context.Background(), "unverified-sub"); !errors.Is(err, user.ErrNotFound) {
		t.Error("an account was created from an unverified Google email")
	}
}

// TestGoogleCallbackRejectsForgedState covers CSRF on the callback: a forged
// call carries no state cookie, so it cannot match.
func TestGoogleCallbackRejectsForgedState(t *testing.T) {
	h, users, _ := testServer(auth.GoogleIdentity{
		Sub:           "attacker-sub",
		Email:         "attacker@example.com",
		EmailVerified: true,
	})

	// No cookies at all, as in a callback forged by another site.
	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=stolen&state=guessed", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	if _, err := users.GetByGoogleID(context.Background(), "attacker-sub"); !errors.Is(err, user.ErrNotFound) {
		t.Error("a forged callback created an account")
	}
}

// TestGoogleCallbackRejectsMismatchedState covers the case where the cookie is
// present but the echoed state does not match it.
func TestGoogleCallbackRejectsMismatchedState(t *testing.T) {
	h, _, _ := testServer(auth.GoogleIdentity{Sub: "s", Email: "a@example.com", EmailVerified: true})

	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=c&state=not-the-cookie", nil)
	req.AddCookie(&http.Cookie{Name: oauthStateCookie, Value: "the-real-state"})
	req.AddCookie(&http.Cookie{Name: oauthVerifierCookie, Value: "v"})

	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}
