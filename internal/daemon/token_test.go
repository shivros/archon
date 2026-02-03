package daemon

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadOrCreateTokenCreates(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "token")

	token, err := LoadOrCreateToken(tokenPath)
	if err != nil {
		t.Fatalf("LoadOrCreateToken: %v", err)
	}
	if token == "" {
		t.Fatalf("expected token")
	}

	data, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatalf("read token file: %v", err)
	}
	fileToken := strings.TrimSpace(string(data))
	if fileToken != token {
		t.Fatalf("file token mismatch")
	}

	decoded, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		t.Fatalf("token not base64: %v", err)
	}
	if len(decoded) != 32 {
		t.Fatalf("expected 32 random bytes, got %d", len(decoded))
	}
}

func TestLoadOrCreateTokenReadsExisting(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "token")

	if err := os.WriteFile(tokenPath, []byte("existing\n"), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}

	token, err := LoadOrCreateToken(tokenPath)
	if err != nil {
		t.Fatalf("LoadOrCreateToken: %v", err)
	}
	if token != "existing" {
		t.Fatalf("expected existing token, got %q", token)
	}
}
