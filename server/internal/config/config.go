package config

import (
	"fmt"
	"os"
	"strings"
)

// Config holds the backend's runtime configuration, sourced from the
// environment. The password hash and JWT signing key live here (config/env),
// never in the database — see the Nexus spec.
type Config struct {
	Addr          string // HTTP listen address, e.g. ":8080"
	DatabaseURL   string // Postgres connection string
	PasswordHash  string // bcrypt hash of the single Owner password
	JWTSigningKey []byte // key used to sign/verify session JWTs
}

// Load reads configuration from the environment, applying defaults where
// sensible and returning an error when a required value is missing.
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

	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); strings.TrimSpace(v) != "" {
		return v
	}
	return fallback
}
