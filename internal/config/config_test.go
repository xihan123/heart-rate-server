package config

import (
	"strings"
	"testing"
)

func setValidEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("DB_DSN", "heartrate.db")
	t.Setenv("COOKIE_HASH_KEY", strings.Repeat("ab", 64))
	t.Setenv("COOKIE_BLOCK_KEY", strings.Repeat("cd", 32))
}

func TestLoadAcceptsRelativeDatabasePath(t *testing.T) {
	setValidEnvironment(t)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if cfg.DBDSN != "heartrate.db" {
		t.Fatalf("database path = %q", cfg.DBDSN)
	}
}

func TestLoadRejectsInvalidCookieKeys(t *testing.T) {
	for _, key := range []string{"COOKIE_HASH_KEY", "COOKIE_BLOCK_KEY"} {
		for _, value := range []string{"", "not-hex", strings.Repeat("ab", 16), strings.Repeat("ab", 65)} {
			t.Run(key+"/"+value, func(t *testing.T) {
				setValidEnvironment(t)
				t.Setenv(key, value)
				if _, err := Load(); err == nil || !strings.Contains(err.Error(), key) {
					t.Fatalf("Load() error = %v", err)
				}
			})
		}
	}
}

func TestValidateRejectsMissingKeysAndDatabase(t *testing.T) {
	setValidEnvironment(t)
	for _, field := range []string{"database", "hash", "block"} {
		t.Run(field, func(t *testing.T) {
			cfg, err := Load()
			if err != nil {
				t.Fatal(err)
			}
			switch field {
			case "database":
				cfg.DBDSN = ""
			case "hash":
				cfg.CookieHashKey = nil
			case "block":
				cfg.CookieBlockKey = nil
			}
			if err := cfg.Validate(); err == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
}
