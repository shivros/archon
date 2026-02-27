package providers

import (
	"reflect"
	"testing"
)

func TestProviderRegistryDefinitions(t *testing.T) {
	tests := []struct {
		name         string
		runtime      Runtime
		candidates   []string
		capabilities Capabilities
	}{
		{
			name:       "codex",
			runtime:    RuntimeCodex,
			candidates: []string{"codex"},
			capabilities: Capabilities{
				SupportsGuidedWorkflowDispatch: true,
				SupportsEvents:                 true,
				SupportsApprovals:              true,
				SupportsInterrupt:              true,
			},
		},
		{
			name:       "claude",
			runtime:    RuntimeClaude,
			candidates: []string{"claude"},
			capabilities: Capabilities{
				SupportsGuidedWorkflowDispatch: true,
				UsesItems:                      true,
				NoProcess:                      true,
			},
		},
		{
			name:       "opencode",
			runtime:    RuntimeOpenCodeServer,
			candidates: nil,
			capabilities: Capabilities{
				SupportsGuidedWorkflowDispatch: true,
				UsesItems:                      true,
				SupportsEvents:                 true,
				SupportsApprovals:              true,
				SupportsInterrupt:              true,
				NoProcess:                      true,
			},
		},
		{
			name:       "kilocode",
			runtime:    RuntimeOpenCodeServer,
			candidates: nil,
			capabilities: Capabilities{
				SupportsGuidedWorkflowDispatch: true,
				UsesItems:                      true,
				SupportsEvents:                 true,
				SupportsApprovals:              true,
				SupportsInterrupt:              true,
				NoProcess:                      true,
			},
		},
		{
			name:       "gemini",
			runtime:    RuntimeExec,
			candidates: []string{"gemini"},
		},
		{
			name:       "custom",
			runtime:    RuntimeCustom,
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

func TestProviderRegistryAllReturnsClones(t *testing.T) {
	defs := All()
	if len(defs) == 0 {
		t.Fatalf("expected providers from registry")
	}
	defs[0].Name = "changed"
	if len(defs[0].CommandCandidates) > 0 {
		defs[0].CommandCandidates[0] = "changed-cmd"
	}

	original, ok := Lookup("codex")
	if !ok {
		t.Fatalf("expected codex definition")
	}
	if original.Name != "codex" {
		t.Fatalf("registry should not be mutated by All() clone edits")
	}
	if len(original.CommandCandidates) == 0 || original.CommandCandidates[0] != "codex" {
		t.Fatalf("command candidates should remain unchanged, got %#v", original.CommandCandidates)
	}
}

func TestProviderRegistryCapabilitiesForKnown(t *testing.T) {
	caps := CapabilitiesFor("codex")
	if !caps.SupportsGuidedWorkflowDispatch || !caps.SupportsEvents || !caps.SupportsApprovals || !caps.SupportsInterrupt {
		t.Fatalf("unexpected codex capabilities: %#v", caps)
	}
}
