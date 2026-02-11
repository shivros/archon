package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCoreConfigDefaults(t *testing.T) {
	t.Setenv("HOME", filepath.Join(t.TempDir(), "home"))
	cfg, err := LoadCoreConfig()
	if err != nil {
		t.Fatalf("LoadCoreConfig: %v", err)
	}
	if cfg.DaemonAddress() != "127.0.0.1:7777" {
		t.Fatalf("unexpected daemon address: %q", cfg.DaemonAddress())
	}
	if cfg.DaemonBaseURL() != "http://127.0.0.1:7777" {
		t.Fatalf("unexpected daemon base url: %q", cfg.DaemonBaseURL())
	}
}

func TestLoadCoreConfigFromTOML(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)

	dataDir := filepath.Join(home, ".archon")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	content := []byte("[daemon]\naddress = \"http://127.0.0.1:9999/\"\n")
	if err := os.WriteFile(filepath.Join(dataDir, "config.toml"), content, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := LoadCoreConfig()
	if err != nil {
		t.Fatalf("LoadCoreConfig: %v", err)
	}
	if cfg.DaemonAddress() != "127.0.0.1:9999" {
		t.Fatalf("unexpected daemon address: %q", cfg.DaemonAddress())
	}
	if cfg.DaemonBaseURL() != "http://127.0.0.1:9999" {
		t.Fatalf("unexpected daemon base url: %q", cfg.DaemonBaseURL())
	}
}

func TestUIConfigResolveKeybindingsPath(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)

	cfg := UIConfig{}
	path, err := cfg.ResolveKeybindingsPath()
	if err != nil {
		t.Fatalf("ResolveKeybindingsPath default: %v", err)
	}
	if want := filepath.Join(home, ".archon", "keybindings.json"); path != want {
		t.Fatalf("unexpected default path: got=%q want=%q", path, want)
	}

	cfg.Keybindings.Path = "ui/keys.json"
	path, err = cfg.ResolveKeybindingsPath()
	if err != nil {
		t.Fatalf("ResolveKeybindingsPath relative: %v", err)
	}
	if want := filepath.Join(home, ".archon", "ui", "keys.json"); path != want {
		t.Fatalf("unexpected relative path: got=%q want=%q", path, want)
	}
}
