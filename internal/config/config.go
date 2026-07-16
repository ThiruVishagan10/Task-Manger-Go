// Package config loads runtime configuration from the environment, optionally
// seeded by a .env file in the working directory.
package config

import (
	"bufio"
	"errors"
	"os"
	"strings"
)

// ErrNoDatabaseURL is returned by Load when no connection string is configured.
var ErrNoDatabaseURL = errors.New("no database URL configured: set Neon_db (or DATABASE_URL) in .env")

const (
	defaultPort    = "8080"
	defaultEnvFile = ".env"
)

// Config holds everything the server needs to start.
type Config struct {
	Port        string
	DatabaseURL string
}

// Load reads configuration from the environment, falling back to a .env file in
// the working directory. It fails when no database URL is set, so that a
// misconfigured server exits at startup rather than on its first request.
func Load() (Config, error) {
	loadDotEnv(defaultEnvFile)

	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	databaseURL := os.Getenv("Neon_db")
	if databaseURL == "" {
		databaseURL = os.Getenv("DATABASE_URL")
	}
	if databaseURL == "" {
		return Config{}, ErrNoDatabaseURL
	}

	return Config{Port: port, DatabaseURL: databaseURL}, nil
}

// loadDotEnv copies KEY=VALUE pairs from path into the environment. A missing
// file is not an error: the environment alone is a valid way to configure the
// server. Existing variables win, so real environment settings are never
// clobbered by a stale .env.
func loadDotEnv(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		value = strings.Trim(value, "\"'")

		if _, exists := os.LookupEnv(key); !exists {
			os.Setenv(key, value)
		}
	}
}
