package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
)

// environment keeps file defaults separate from exported environment variables.
// Reading configuration never mutates the process environment.
type environment map[string]string

func (e environment) get(key string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return e[key]
}

func loadEnvironment() (environment, error) {
	explicit := strings.TrimSpace(os.Getenv("NEXUS_ENV_FILE"))
	paths := []string{".env"}
	if explicit != "" {
		paths = []string{explicit}
	} else {
		dir, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("locate env file: %w", err)
		}
		// The documented development command starts in server/. Only this
		// directory gets a parent fallback; do not scan arbitrary ancestors.
		if filepath.Base(dir) == "server" {
			paths = append(paths, filepath.Join("..", ".env"))
		}
	}

	for _, name := range paths {
		data, err := os.ReadFile(name)
		if err != nil {
			if explicit == "" && os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read env file %q: %w", name, err)
		}
		values, err := godotenv.UnmarshalBytes(data)
		if err != nil {
			// Parser errors may contain the original line, including password
			// hashes and signing keys. Never include them in startup logs.
			return nil, fmt.Errorf("parse env file %q: invalid dotenv syntax (check KEY=value and quoting)", name)
		}
		return environment(values), nil
	}
	return nil, nil
}
