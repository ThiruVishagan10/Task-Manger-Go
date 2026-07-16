// Package config loads runtime configuration from the environment, optionally
// seeded by a .env file in the working directory.
package config

import (
	"bufio"
	"errors"
	"os"
	"strconv"
	"strings"
)

// ErrNoDatabaseURL is returned by Load when no connection string is configured.
var ErrNoDatabaseURL = errors.New("no database URL configured: set Neon_db (or DATABASE_URL) in .env")

const (
	defaultPort              = "8080"
	defaultEnvFile           = ".env"
	defaultGoogleRedirectURL = "http://localhost:8080/auth/google/callback"
	defaultPostLoginRedirect = "/"
)

// Config holds everything the server needs to start.
type Config struct {
	Port        string
	DatabaseURL string
	Google      GoogleConfig

	// SecureCookies marks session cookies Secure, so browsers withhold them
	// from plaintext HTTP. It must be on in production and off for local
	// development over http://localhost, where a Secure cookie would never be
	// sent back and login would appear to silently fail.
	SecureCookies bool

	// PostLoginRedirect is where a browser lands after Google sign-in
	// completes. It comes from configuration and never from the request, so it
	// cannot be turned into an open redirect.
	PostLoginRedirect string
}

// GoogleConfig holds the OAuth 2.0 client credentials for Google sign-in.
type GoogleConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

// Enabled reports whether Google sign-in is configured. It is optional: the
// server runs with email and password alone, and only refuses the Google routes.
func (g GoogleConfig) Enabled() bool {
	return g.ClientID != "" && g.ClientSecret != ""
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

	redirectURL := os.Getenv("GOOGLE_REDIRECT_URL")
	if redirectURL == "" {
		redirectURL = defaultGoogleRedirectURL
	}

	postLoginRedirect := os.Getenv("POST_LOGIN_REDIRECT")
	if postLoginRedirect == "" {
		postLoginRedirect = defaultPostLoginRedirect
	}

	// An unparseable value is treated as unset rather than fatal, matching how
	// the rest of this loader degrades to defaults.
	secureCookies, _ := strconv.ParseBool(os.Getenv("COOKIE_SECURE"))

	return Config{
		Port:        port,
		DatabaseURL: databaseURL,
		Google: GoogleConfig{
			ClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
			ClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
			RedirectURL:  redirectURL,
		},
		SecureCookies:     secureCookies,
		PostLoginRedirect: postLoginRedirect,
	}, nil
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
