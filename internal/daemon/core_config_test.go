package daemon

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCoreConfigOrDefaultFallsBackOnError(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	dataDir := filepath.Join(home, ".archon")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "config.toml"), []byte("[daemon\naddress='bad'"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg := loadCoreConfigOrDefault()
	if cfg.DaemonAddress() != "127.0.0.1:7777" {
		t.Fatalf("expected default daemon address, got %q", cfg.DaemonAddress())
	}
}
