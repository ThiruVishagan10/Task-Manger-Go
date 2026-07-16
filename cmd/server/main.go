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
	"github.com/ThiruVishagan10/Task-Manger-Go/internal/config"
	"github.com/ThiruVishagan10/Task-Manger-Go/internal/database"
	"github.com/ThiruVishagan10/Task-Manger-Go/internal/task"
)

const (
	// shutdownTimeout bounds how long in-flight requests may finish once a
	// shutdown signal arrives.
	shutdownTimeout = 10 * time.Second

	// readHeaderTimeout guards against clients that open a connection and then
	// dribble out headers to hold it open.
	readHeaderTimeout = 5 * time.Second
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

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           api.NewServer(task.NewStore(pool)).Routes(),
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
