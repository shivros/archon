package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"control/internal/providers"
)

func TestResolveProviderUsesRegistryDefinitions(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	dataDir := filepath.Join(home, ".archon")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	config := []byte(`
[providers.codex]
command = "` + os.Args[0] + `"

[providers.claude]
command = "` + os.Args[0] + `"

[providers.opencode]
command = "` + os.Args[0] + `"
base_url = "http://127.0.0.1:4096"

[providers.kilocode]
command = "` + os.Args[0] + `"
base_url = "http://127.0.0.1:4097"

[providers.gemini]
command = "` + os.Args[0] + `"
`)
	if err := os.WriteFile(filepath.Join(dataDir, "config.toml"), config, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	tests := []struct {
		name      string
		customCmd string
	}{
		{name: "codex"},
		{name: "claude"},
		{name: "opencode"},
		{name: "kilocode"},
		{name: "gemini"},
		{name: "custom", customCmd: os.Args[0]},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := ResolveProvider(tt.name, tt.customCmd)
			if err != nil {
				t.Fatalf("ResolveProvider(%q) error: %v", tt.name, err)
			}
			if !strings.EqualFold(provider.Name(), tt.name) {
				t.Fatalf("expected provider name %q, got %q", tt.name, provider.Name())
			}
			if strings.TrimSpace(provider.Command()) == "" {
				t.Fatalf("expected non-empty provider command")
			}
		})
	}
}

func TestResolveProviderCustomRequiresCommand(t *testing.T) {
	_, err := ResolveProvider("custom", "")
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "requires cmd") {
		t.Fatalf("expected custom command requirement error, got %v", err)
	}
}

func TestResolveProviderConfigLoadError(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	dataDir := filepath.Join(home, ".archon")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "config.toml"), []byte("[providers.codex\ncommand='x'"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err := ResolveProvider("codex", "")
	if err == nil {
		t.Fatalf("expected config parse error")
	}
}

func TestResolveProviderOpenCodeDoesNotRequireCommandLookup(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	dataDir := filepath.Join(home, ".archon")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	content := []byte(`
[providers.opencode]
base_url = "http://127.0.0.1:4096"
`)
	if err := os.WriteFile(filepath.Join(dataDir, "config.toml"), content, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("PATH", filepath.Join(t.TempDir(), "missing-bin"))

	provider, err := ResolveProvider("opencode", "")
	if err != nil {
		t.Fatalf("ResolveProvider(opencode): %v", err)
	}
	if provider.Command() != "http://127.0.0.1:4096" {
		t.Fatalf("expected server base url command view, got %q", provider.Command())
	}
}

func TestResolveProviderRequiresProviderName(t *testing.T) {
	if _, err := ResolveProvider("   ", ""); err == nil {
		t.Fatalf("expected provider validation error")
	}
}

func TestResolveProviderUnknownProvider(t *testing.T) {
	if _, err := ResolveProvider("unknown-provider", ""); err == nil {
		t.Fatalf("expected unknown provider error")
	}
}

func TestResolveProviderFactoryMissingForRuntime(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	dataDir := filepath.Join(home, ".archon")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	content := []byte(`
[providers.gemini]
command = "` + os.Args[0] + `"
`)
	if err := os.WriteFile(filepath.Join(dataDir, "config.toml"), content, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	originalFactory := providerFactories[providers.RuntimeExec]
	providerFactories[providers.RuntimeExec] = nil
	defer func() {
		providerFactories[providers.RuntimeExec] = originalFactory
	}()

	if _, err := ResolveProvider("gemini", ""); err == nil || !strings.Contains(strings.ToLower(err.Error()), "not supported") {
		t.Fatalf("expected unsupported runtime error, got %v", err)
	}
}

func TestResolveProviderCommandNameRequiresCandidates(t *testing.T) {
	def := providers.Definition{Name: "x", Runtime: providers.RuntimeExec}
	if _, err := resolveProviderCommandName(def, ""); err == nil || !strings.Contains(strings.ToLower(err.Error()), "not configured") {
		t.Fatalf("expected missing command error, got %v", err)
	}
}

func TestResolveProviderCommandNameReportsAllCandidates(t *testing.T) {
	def := providers.Definition{
		Name:              "x",
		Runtime:           providers.RuntimeExec,
		CommandCandidates: []string{"missing-a", "", "missing-b"},
	}
	_, err := resolveProviderCommandName(def, "")
	if err == nil {
		t.Fatalf("expected command lookup error")
	}
	text := strings.ToLower(err.Error())
	if !strings.Contains(text, "missing-a") || !strings.Contains(text, "missing-b") {
		t.Fatalf("expected candidate names in error, got %v", err)
	}
}

func TestResolveProviderCommandNameUsesConfigOverride(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	dataDir := filepath.Join(home, ".archon")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	content := []byte(`
[providers.opencode]
command = "` + os.Args[0] + `"
`)
	if err := os.WriteFile(filepath.Join(dataDir, "config.toml"), content, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	def, ok := providers.Lookup("opencode")
	if !ok {
		t.Fatalf("expected opencode definition")
	}
	cmd, err := resolveProviderCommandName(def, "")
	if err != nil {
		t.Fatalf("resolveProviderCommandName: %v", err)
	}
	if cmd != os.Args[0] {
		t.Fatalf("expected config override command, got %q", cmd)
	}
}
