package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPortableManagerPersistence(t *testing.T) {
	runtimeDir := os.Getenv("ARTEX_TEST_POSTGRES_DIR")
	if runtimeDir == "" {
		t.Skip("requires the unpacked portable runtime")
	}
	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configPath, []byte(`{"database":{"driver":"embedded"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ARTEX_CONFIG", configPath)
	t.Setenv("ARTEX_PG_DSN", "")
	t.Setenv("ARTEX_POSTGRES_DIR", runtimeDir)
	data := filepath.Join(t.TempDir(), "data with spaces")
	m, err := NewManager(data, "")
	if err != nil {
		t.Fatal(err)
	}
	first := m
	t.Cleanup(func() { _ = first.Close() })
	if m.localPG == nil {
		t.Fatal("portable configuration did not select the bundled database")
	}
	if err := m.pg.SetSetting("portable_persistence_test", "saved"); err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	m, err = NewManager(data, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close() })
	if value, ok, err := m.pg.GetSetting("portable_persistence_test"); err != nil || !ok || value != "saved" {
		t.Fatalf("portable restart lost settings: value=%q ok=%v err=%v", value, ok, err)
	}
}
