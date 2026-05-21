package client

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func setTestHomeDir(t *testing.T, home string) {
	t.Helper()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
}

func resetStreamDebugStateForTest() {
	streamDebug = false
	streamDebugOnce = sync.Once{}
	streamLogger = nil
	streamLoggerOnce = sync.Once{}
}

func TestStreamDebugEnabledDefaultsFalse(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	setTestHomeDir(t, home)
	resetStreamDebugStateForTest()
	if streamDebugEnabled() {
		t.Fatalf("expected stream debug disabled by default")
	}
}

func TestStreamDebugEnabledFromCoreConfig(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	setTestHomeDir(t, home)
	dataDir := filepath.Join(home, ".archon")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	content := []byte("[debug]\nstream_debug = true\n")
	if err := os.WriteFile(filepath.Join(dataDir, "config.toml"), content, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	resetStreamDebugStateForTest()
	if !streamDebugEnabled() {
		t.Fatalf("expected stream debug enabled from config")
	}
}
