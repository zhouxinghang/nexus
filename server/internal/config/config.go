package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds the backend's runtime configuration, sourced from the
// environment. The password hash and JWT signing key live here (config/env),
// never in the database — see the Nexus spec.
type Config struct {
	Addr          string // HTTP listen address, e.g. ":8080"
	DatabaseURL   string // Postgres connection string
	PasswordHash  string // bcrypt hash of the single Owner password
	JWTSigningKey []byte // key used to sign/verify session JWTs

	CookieSecure bool // mark the session cookie Secure (HTTPS-only)
	TrustedProxy bool // honor X-Forwarded-For / X-Real-IP for the client IP

	LoginMaxFailures int           // failed logins before a lockout
	LoginWindow      time.Duration // window in which failures are counted
	LoginLockout     time.Duration // how long a locked-out client waits
}

// Load reads configuration from the environment, applying defaults where
// sensible and returning an error when a required value is missing.
//
// Required: NEXUS_DATABASE_URL, NEXUS_PASSWORD_HASH, NEXUS_JWT_SECRET.
// Optional (default):
//   - NEXUS_ADDR (":8080")
//   - NEXUS_COOKIE_SECURE (true) — set false for plain-HTTP local dev
//   - NEXUS_TRUSTED_PROXY (false) — set true only behind a reverse proxy
//   - NEXUS_LOGIN_MAX_FAILURES (5)
//   - NEXUS_LOGIN_WINDOW (15m) — Go duration
//   - NEXUS_LOGIN_LOCKOUT (1m) — Go duration
func Load() (Config, error) {
	cfg := Config{
		Addr:         envOr("NEXUS_ADDR", ":8080"),
		DatabaseURL:  os.Getenv("NEXUS_DATABASE_URL"),
		PasswordHash: os.Getenv("NEXUS_PASSWORD_HASH"),
	}

	key := os.Getenv("NEXUS_JWT_SECRET")
	if strings.TrimSpace(key) == "" {
		return Config{}, fmt.Errorf("NEXUS_JWT_SECRET is required")
	}
	cfg.JWTSigningKey = []byte(key)

	if strings.TrimSpace(cfg.DatabaseURL) == "" {
		return Config{}, fmt.Errorf("NEXUS_DATABASE_URL is required")
	}
	if strings.TrimSpace(cfg.PasswordHash) == "" {
		return Config{}, fmt.Errorf("NEXUS_PASSWORD_HASH is required")
	}

	var err error
	if cfg.CookieSecure, err = envBool("NEXUS_COOKIE_SECURE", true); err != nil {
		return Config{}, err
	}
	if cfg.TrustedProxy, err = envBool("NEXUS_TRUSTED_PROXY", false); err != nil {
		return Config{}, err
	}
	if cfg.LoginMaxFailures, err = envInt("NEXUS_LOGIN_MAX_FAILURES", 5); err != nil {
		return Config{}, err
	}
	if cfg.LoginWindow, err = envDuration("NEXUS_LOGIN_WINDOW", 15*time.Minute); err != nil {
		return Config{}, err
	}
	if cfg.LoginLockout, err = envDuration("NEXUS_LOGIN_LOCKOUT", time.Minute); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); strings.TrimSpace(v) != "" {
		return v
	}
	return fallback
}

func envBool(key string, fallback bool) (bool, error) {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean: %w", key, err)
	}
	return b, nil
}

func envInt(key string, fallback int) (int, error) {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return n, nil
}

func envDuration(key string, fallback time.Duration) (time.Duration, error) {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration like 1m", key)
	}
	return d, nil
}
