package daemon

import (
	"os"
	"strings"
	"testing"
)

func TestResolveProviderUsesRegistryDefinitions(t *testing.T) {
	t.Setenv("ARCHON_CODEX_CMD", os.Args[0])
	t.Setenv("ARCHON_CLAUDE_CMD", os.Args[0])
	t.Setenv("ARCHON_OPENCODE_CMD", os.Args[0])
	t.Setenv("ARCHON_GEMINI_CMD", os.Args[0])

	tests := []struct {
		name      string
		customCmd string
	}{
		{name: "codex"},
		{name: "claude"},
		{name: "opencode"},
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
