package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUseLocalPostgres(t *testing.T) {
	t.Setenv("ARTEX_PG_DSN", "")
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("ARTEX_CONFIG", path)
	for _, tt := range []struct {
		name, config string
		local        bool
	}{
		{"empty", `{}`, true},
		{"portable", `{"database":{"driver":"embedded"}}`, true},
		{"explicit external", `{"database":{"driver":"postgres"}}`, false},
		{"external dsn", `{"database":{"dsn":"postgres://host/db"}}`, false},
		{"external fields", `{"database":{"host":"localhost","dbname":"artex"}}`, false},
		{"unsupported sqlite", `{"database":{"driver":"sqlite","dsn":"file::memory:?cache=shared"}}`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(tt.config), 0600); err != nil {
				t.Fatal(err)
			}
			if got := UseLocalPostgres(); got != tt.local {
				t.Fatalf("UseLocalPostgres() = %v, want %v", got, tt.local)
			}
		})
	}
	if err := os.WriteFile(path, []byte(`{"database":{"driver":"embedded"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ARTEX_PG_DSN", "postgres://external/db")
	if UseLocalPostgres() {
		t.Fatal("explicit environment DSN must override embedded configuration")
	}
}

func TestPostgresDSNRejectsSQLite(t *testing.T) {
	t.Setenv("ARTEX_PG_DSN", "")
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"database":{"driver":"sqlite","dsn":"file::memory:?cache=shared"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ARTEX_CONFIG", path)
	if _, _, err := PostgresDSN(); err == nil {
		t.Fatal("unsupported database driver must fail before attempting a connection")
	}
}
