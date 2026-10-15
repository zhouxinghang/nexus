package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fileConfig = `# Local runtime configuration
export NEXUS_DATABASE_URL='postgresql://owner:fixture@localhost:5432/nexus'
NEXUS_PASSWORD_HASH='$2y$10$test-hash-with-dollar-signs'
NEXUS_JWT_SECRET='fixture-signing-key'
NEXUS_ADDR=:9090 # inline comments are allowed
NEXUS_COOKIE_SECURE=false
NEXUS_TRUSTED_PROXY=true
NEXUS_LOGIN_MAX_FAILURES=8
NEXUS_LOGIN_WINDOW=20m
NEXUS_LOGIN_LOCKOUT=2m
`

// Configuration must be isolated from any credentials in the test runner's
// environment. Load must not export file values into the parent process.
func clearNexusEnvironment(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"NEXUS_ENV_FILE", "NEXUS_DATABASE_URL", "NEXUS_PASSWORD_HASH",
		"NEXUS_JWT_SECRET", "NEXUS_ADDR", "NEXUS_COOKIE_SECURE",
		"NEXUS_TRUSTED_PROXY", "NEXUS_LOGIN_MAX_FAILURES",
		"NEXUS_LOGIN_WINDOW", "NEXUS_LOGIN_LOCKOUT",
	} {
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
}

func writeEnv(t *testing.T, name, contents string) {
	t.Helper()
	if err := os.WriteFile(name, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
}

func assertFileConfig(t *testing.T, cfg Config) {
	t.Helper()
	if cfg.DatabaseURL != "postgresql://owner:fixture@localhost:5432/nexus" ||
		cfg.PasswordHash != "$2y$10$test-hash-with-dollar-signs" ||
		string(cfg.JWTSigningKey) != "fixture-signing-key" {
		t.Fatal("required file configuration did not load or a quoted hash was expanded")
	}
	if cfg.Addr != ":9090" || cfg.CookieSecure || !cfg.TrustedProxy ||
		cfg.LoginMaxFailures != 8 || cfg.LoginWindow.Minutes() != 20 ||
		cfg.LoginLockout.Minutes() != 2 {
		t.Fatal("optional file configuration did not load")
	}
}

func TestLoadDotEnv(t *testing.T) {
	for _, directory := range []string{"root", "server"} {
		t.Run(directory, func(t *testing.T) {
			clearNexusEnvironment(t)
			root := t.TempDir()
			writeEnv(t, filepath.Join(root, ".env"), fileConfig)
			workdir := root
			if directory == "server" {
				workdir = filepath.Join(root, "server")
				if err := os.Mkdir(workdir, 0700); err != nil {
					t.Fatal(err)
				}
			}
			t.Chdir(workdir)
			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			assertFileConfig(t, cfg)
			if _, ok := os.LookupEnv("NEXUS_JWT_SECRET"); ok {
				t.Fatal("loading configuration must not mutate the process environment")
			}
		})
	}
}

func TestLoadPrefersCurrentDirectoryFileWithoutMerging(t *testing.T) {
	clearNexusEnvironment(t)
	root := t.TempDir()
	writeEnv(t, filepath.Join(root, ".env"), fileConfig)
	server := filepath.Join(root, "server")
	if err := os.Mkdir(server, 0700); err != nil {
		t.Fatal(err)
	}
	writeEnv(t, filepath.Join(server, ".env"), "NEXUS_ADDR=:7070\n")
	t.Chdir(server)
	if _, err := Load(); err == nil || err.Error() != "NEXUS_JWT_SECRET is required" {
		t.Fatal("a local .env must not silently inherit secrets from a parent .env")
	}
}

func TestLoadEnvironmentWinsOverFile(t *testing.T) {
	clearNexusEnvironment(t)
	dir := t.TempDir()
	t.Chdir(dir)
	writeEnv(t, ".env", fileConfig)
	t.Setenv("NEXUS_JWT_SECRET", "shell-signing-key")
	t.Setenv("NEXUS_ADDR", ":8081")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if string(cfg.JWTSigningKey) != "shell-signing-key" || cfg.Addr != ":8081" {
		t.Fatal("an exported environment variable must override the file")
	}
	// Explicitly empty environment variables also take priority; required
	// configuration must fail instead of falling back to another secret.
	t.Setenv("NEXUS_JWT_SECRET", "")
	if _, err := Load(); err == nil || err.Error() != "NEXUS_JWT_SECRET is required" {
		t.Fatal("an explicitly empty exported secret must not use the file secret")
	}
}

func TestLoadExplicitEnvFile(t *testing.T) {
	clearNexusEnvironment(t)
	root := t.TempDir()
	target := filepath.Join(root, "selected.env")
	writeEnv(t, target, fileConfig)
	workdir := t.TempDir()
	t.Chdir(workdir)
	writeEnv(t, ".env", "NEXUS_JWT_SECRET='wrong-file'\n")
	t.Setenv("NEXUS_ENV_FILE", target)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	assertFileConfig(t, cfg)

	// Relative paths resolve against the startup working directory.
	writeEnv(t, "selected.env", fileConfig)
	t.Setenv("NEXUS_ENV_FILE", "selected.env")
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	assertFileConfig(t, cfg)
}

func TestLoadMissingOptionalFileUsesEnvironmentAndDefaults(t *testing.T) {
	clearNexusEnvironment(t)
	t.Chdir(t.TempDir())
	t.Setenv("NEXUS_DATABASE_URL", "postgresql://owner:fixture@localhost:5432/nexus")
	t.Setenv("NEXUS_PASSWORD_HASH", "fixture-hash")
	t.Setenv("NEXUS_JWT_SECRET", "fixture-signing-key")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != ":8080" || !cfg.CookieSecure || cfg.TrustedProxy ||
		cfg.LoginMaxFailures != 5 || cfg.LoginWindow.Minutes() != 15 ||
		cfg.LoginLockout.Minutes() != 1 {
		t.Fatal("environment-only startup defaults changed")
	}
}

func TestLoadDoesNotSearchUnrelatedParentDirectories(t *testing.T) {
	clearNexusEnvironment(t)
	root := t.TempDir()
	writeEnv(t, filepath.Join(root, ".env"), fileConfig)
	dir := filepath.Join(root, "unrelated")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	if _, err := Load(); err == nil || err.Error() != "NEXUS_JWT_SECRET is required" {
		t.Fatal("unrelated parent directories must not supply credentials")
	}
}

func TestLoadEnvFileErrorsDoNotExposeContents(t *testing.T) {
	for _, scenario := range []string{"missing-explicit", "malformed", "directory", "malformed-parent"} {
		t.Run(scenario, func(t *testing.T) {
			clearNexusEnvironment(t)
			root := t.TempDir()
			t.Chdir(root)
			// Even valid exported settings must not hide errors in a chosen file.
			t.Setenv("NEXUS_DATABASE_URL", "postgresql://owner:fixture@localhost:5432/nexus")
			t.Setenv("NEXUS_PASSWORD_HASH", "fixture-hash")
			t.Setenv("NEXUS_JWT_SECRET", "shell-signing-key")
			const sensitive = "fixture-secret-that-must-not-appear-in-errors"
			switch scenario {
			case "missing-explicit":
				t.Setenv("NEXUS_ENV_FILE", filepath.Join(root, "missing.env"))
			case "malformed", "malformed-parent":
				writeEnv(t, ".env", "NEXUS_JWT_SECRET='"+sensitive)
				if scenario == "malformed-parent" {
					server := filepath.Join(root, "server")
					if err := os.Mkdir(server, 0700); err != nil {
						t.Fatal(err)
					}
					t.Chdir(server)
				}
			case "directory":
				if err := os.Mkdir(".env", 0700); err != nil {
					t.Fatal(err)
				}
			}
			_, err := Load()
			if err == nil {
				t.Fatal("chosen-file errors must abort startup")
			}
			if strings.Contains(err.Error(), sensitive) {
				t.Fatal("configuration error exposed file contents")
			}
		})
	}
}

func TestLoadValidatesFileSettingsWithoutExposingValues(t *testing.T) {
	for _, key := range []string{
		"NEXUS_COOKIE_SECURE", "NEXUS_TRUSTED_PROXY", "NEXUS_LOGIN_MAX_FAILURES",
		"NEXUS_LOGIN_WINDOW", "NEXUS_LOGIN_LOCKOUT",
	} {
		t.Run(key, func(t *testing.T) {
			clearNexusEnvironment(t)
			t.Chdir(t.TempDir())
			const sensitive = "fixture-private-value"
			writeEnv(t, ".env", fileConfig+key+"='"+sensitive+"'\n")
			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), key) {
				t.Fatal("invalid file setting must identify the configuration key")
			}
			if strings.Contains(err.Error(), sensitive) {
				t.Fatal("validation error must not include configuration values")
			}
		})
	}
}
