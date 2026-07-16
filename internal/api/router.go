// Package api exposes the task store over HTTP.
package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/ThiruVishagan10/Task-Manger-Go/internal/auth"
	"github.com/ThiruVishagan10/Task-Manger-Go/internal/task"
	"github.com/ThiruVishagan10/Task-Manger-Go/internal/user"
)

// The persistence and identity behaviour the API depends on. Each is declared
// here, at the point of use, so that handlers can be exercised against fakes
// without a database or a round trip to Google.

// TaskStore persists tasks. Every method takes the owning user's ID: tasks are
// private, and threading the owner through the store rather than filtering
// afterwards means an ownership check cannot be forgotten at a call site.
type TaskStore interface {
	Create(ctx context.Context, userID int, t *task.Task) error
	List(ctx context.Context, userID int) ([]task.Task, error)
	Get(ctx context.Context, userID, id int) (task.Task, error)
	Update(ctx context.Context, userID, id int, t task.Task) (task.Task, error)
	Delete(ctx context.Context, userID, id int) error
}

// UserStore persists accounts.
type UserStore interface {
	Create(ctx context.Context, u *user.User) error
	GetByID(ctx context.Context, id int) (user.User, error)
	GetByEmail(ctx context.Context, email string) (user.User, error)
	GetByGoogleID(ctx context.Context, googleID string) (user.User, error)
	LinkGoogleID(ctx context.Context, id int, googleID string) error
}

// SessionStore issues and resolves login sessions.
type SessionStore interface {
	Create(ctx context.Context, userID int) (auth.Session, error)
	UserID(ctx context.Context, token string) (int, error)
	Delete(ctx context.Context, token string) error
}

// GoogleAuthenticator performs the Google OAuth 2.0 authorization code flow.
type GoogleAuthenticator interface {
	AuthCodeURL(state, codeChallenge string) string
	Exchange(ctx context.Context, code, codeVerifier string) (auth.GoogleIdentity, error)
}

// Deps is everything a Server needs. It is a struct rather than a parameter
// list because the wiring is long enough that names at the call site are worth
// more than brevity.
type Deps struct {
	Tasks    TaskStore
	Users    UserStore
	Sessions SessionStore

	// Google is nil when Google sign-in is not configured, in which case the
	// Google routes answer 501 rather than disappearing — an operator who has
	// misconfigured the credentials gets told so, instead of a 404 that looks
	// like a client bug.
	Google GoogleAuthenticator

	SecureCookies     bool
	PostLoginRedirect string
}

// Server holds the dependencies shared by every handler.
type Server struct {
	tasks    TaskStore
	users    UserStore
	sessions SessionStore
	google   GoogleAuthenticator

	secureCookies     bool
	postLoginRedirect string
}

// NewServer returns a Server backed by deps.
func NewServer(deps Deps) *Server {
	return &Server{
		tasks:             deps.Tasks,
		users:             deps.Users,
		sessions:          deps.Sessions,
		google:            deps.Google,
		secureCookies:     deps.SecureCookies,
		postLoginRedirect: deps.PostLoginRedirect,
	}
}

// Routes returns a handler serving every endpoint of the API.
//
// Patterns carry their method, so ServeMux answers unsupported methods with 405
// and an Allow header on its own.
//
// Every task route is wrapped in requireAuth. That wrapping is the only thing
// standing between a task and the internet, so a new task route must be added
// with it.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", s.handleWelcome)

	mux.HandleFunc("POST /auth/register", s.handleRegister)
	mux.HandleFunc("POST /auth/login", s.handleLogin)
	mux.HandleFunc("POST /auth/logout", s.handleLogout)
	mux.HandleFunc("GET /auth/me", s.requireAuth(s.handleMe))

	mux.HandleFunc("GET /auth/google", s.handleGoogleStart)
	mux.HandleFunc("GET /auth/google/callback", s.handleGoogleCallback)

	mux.HandleFunc("GET /tasks", s.requireAuth(s.handleListTasks))
	mux.HandleFunc("POST /tasks", s.requireAuth(s.handleCreateTask))

	mux.HandleFunc("GET /tasks/{id}", s.requireAuth(s.handleGetTask))
	mux.HandleFunc("PUT /tasks/{id}", s.requireAuth(s.handleUpdateTask))
	mux.HandleFunc("DELETE /tasks/{id}", s.requireAuth(s.handleDeleteTask))

	return mux
}

// handleWelcome reports that the API is up.
func (s *Server) handleWelcome(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "Welcome to the Task Manager API!")
}
