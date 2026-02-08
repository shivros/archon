package providers

import (
	"reflect"
	"testing"
)

func TestProviderRegistryDefinitions(t *testing.T) {
	tests := []struct {
		name         string
		runtime      Runtime
		commandEnv   string
		candidates   []string
		capabilities Capabilities
	}{
		{
			name:       "codex",
			runtime:    RuntimeCodex,
			commandEnv: "ARCHON_CODEX_CMD",
			candidates: []string{"codex"},
			capabilities: Capabilities{
				SupportsEvents:    true,
				SupportsApprovals: true,
				SupportsInterrupt: true,
			},
		},
		{
			name:       "claude",
			runtime:    RuntimeClaude,
			commandEnv: "ARCHON_CLAUDE_CMD",
			candidates: []string{"claude"},
			capabilities: Capabilities{
				UsesItems: true,
				NoProcess: true,
			},
		},
		{
			name:       "opencode",
			runtime:    RuntimeExec,
			commandEnv: "ARCHON_OPENCODE_CMD",
			candidates: []string{"opencode", "opencode-cli"},
		},
		{
			name:       "gemini",
			runtime:    RuntimeExec,
			commandEnv: "ARCHON_GEMINI_CMD",
			candidates: []string{"gemini"},
		},
		{
			name:       "custom",
			runtime:    RuntimeCustom,
			commandEnv: "",
			candidates: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def, ok := Lookup(tt.name)
			if !ok {
				t.Fatalf("expected provider %q to be registered", tt.name)
			}
			if def.Name != tt.name {
				t.Fatalf("expected name %q, got %q", tt.name, def.Name)
			}
			if def.Runtime != tt.runtime {
				t.Fatalf("expected runtime %q, got %q", tt.runtime, def.Runtime)
			}
			if def.CommandEnv != tt.commandEnv {
				t.Fatalf("expected command env %q, got %q", tt.commandEnv, def.CommandEnv)
			}
			if !reflect.DeepEqual(def.CommandCandidates, tt.candidates) {
				t.Fatalf("expected candidates %#v, got %#v", tt.candidates, def.CommandCandidates)
			}
			if def.Capabilities != tt.capabilities {
				t.Fatalf("expected capabilities %#v, got %#v", tt.capabilities, def.Capabilities)
			}
		})
	}
}

func TestProviderRegistryNormalizeAndLookup(t *testing.T) {
	def, ok := Lookup("  CODEX ")
	if !ok {
		t.Fatalf("expected normalized lookup to succeed")
	}
	if def.Name != "codex" {
		t.Fatalf("expected codex provider, got %q", def.Name)
	}
	if Normalize("  Claude ") != "claude" {
		t.Fatalf("unexpected normalization")
	}
}

func TestProviderRegistryUnknown(t *testing.T) {
	if _, ok := Lookup("unknown-provider"); ok {
		t.Fatalf("expected unknown provider lookup to fail")
	}
	if caps := CapabilitiesFor("unknown-provider"); caps != (Capabilities{}) {
		t.Fatalf("expected empty capabilities for unknown provider, got %#v", caps)
	}
}
