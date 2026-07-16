// Command server runs the Task Manager HTTP API.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ThiruVishagan10/Task-Manger-Go/internal/api"
	"github.com/ThiruVishagan10/Task-Manger-Go/internal/auth"
	"github.com/ThiruVishagan10/Task-Manger-Go/internal/config"
	"github.com/ThiruVishagan10/Task-Manger-Go/internal/database"
	"github.com/ThiruVishagan10/Task-Manger-Go/internal/task"
	"github.com/ThiruVishagan10/Task-Manger-Go/internal/user"
)

const (
	// shutdownTimeout bounds how long in-flight requests may finish once a
	// shutdown signal arrives.
	shutdownTimeout = 10 * time.Second

	// readHeaderTimeout guards against clients that open a connection and then
	// dribble out headers to hold it open.
	readHeaderTimeout = 5 * time.Second

	// sessionSweepInterval is how often expired sessions are collected. Expired
	// sessions are already refused at lookup, so this only keeps the table from
	// growing without bound and can afford to be infrequent.
	sessionSweepInterval = time.Hour
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)

	if err := run(); err != nil {
		log.Fatal(err)
	}
}

// run wires the application together and blocks until the server stops. It
// returns an error rather than calling log.Fatal so that deferred cleanup —
// closing the connection pool — always runs.
func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := database.Migrate(ctx, pool); err != nil {
		return err
	}

	log.Println("Connected to database")

	sessions := auth.NewSessionStore(pool)

	go sweepExpiredSessions(ctx, sessions)

	deps := api.Deps{
		Tasks:             task.NewStore(pool),
		Users:             user.NewStore(pool),
		Sessions:          sessions,
		Google:            googleAuthenticator(cfg.Google),
		SecureCookies:     cfg.SecureCookies,
		PostLoginRedirect: cfg.PostLoginRedirect,
	}

	logAuthMethods(cfg)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           api.NewServer(deps).Routes(),
		ReadHeaderTimeout: readHeaderTimeout,
	}

	// ListenAndServe blocks, so run it alongside the signal watch and take
	// whichever finishes first.
	serveErr := make(chan error, 1)
	go func() {
		log.Printf("Server running on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- fmt.Errorf("serve: %w", err)
			return
		}
		serveErr <- nil
	}()

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
		log.Println("Shutdown signal received, draining requests...")
	}

	// A fresh context: ctx is already cancelled by the signal.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}

	log.Println("Server stopped")

	return nil
}

// googleAuthenticator returns the Google sign-in client, or nil when no
// credentials are configured.
//
// Google sign-in is optional: a nil client leaves the rest of the API working
// on email and password alone, and the Google routes report that they are
// unconfigured. Returning api.GoogleAuthenticator rather than *auth.GoogleClient
// matters — a nil *auth.GoogleClient stored in an interface is not a nil
// interface, and the handlers' nil check would miss it.
func googleAuthenticator(cfg config.GoogleConfig) api.GoogleAuthenticator {
	if !cfg.Enabled() {
		return nil
	}

	return auth.NewGoogleClient(cfg.ClientID, cfg.ClientSecret, cfg.RedirectURL)
}

// sweepExpiredSessions deletes timed-out sessions until ctx is cancelled.
//
// A failed sweep is logged rather than fatal: stale rows are untidy, not
// dangerous, since expiry is enforced on every lookup regardless.
func sweepExpiredSessions(ctx context.Context, sessions *auth.SessionStore) {
	ticker := time.NewTicker(sessionSweepInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			deleted, err := sessions.DeleteExpired(ctx)
			if err != nil {
				// Cancellation during shutdown surfaces here as a query
				// failure; it is not worth reporting as one.
				if ctx.Err() == nil {
					log.Printf("[WARN] sweep expired sessions: %v", err)
				}
				continue
			}

			if deleted > 0 {
				log.Printf("Collected %d expired session(s)", deleted)
			}
		}
	}
}

// logAuthMethods reports which sign-in methods came up, and warns about
// settings that are right for local development and wrong in production.
func logAuthMethods(cfg config.Config) {
	if cfg.Google.Enabled() {
		log.Printf("Google sign-in enabled, redirecting to %s", cfg.Google.RedirectURL)
	} else {
		log.Println("Google sign-in disabled: set GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET to enable it")
	}

	if !cfg.SecureCookies {
		log.Println("[WARN] Session cookies are not marked Secure, so they will travel over plain HTTP. Set COOKIE_SECURE=true when serving over HTTPS.")
	}
}
