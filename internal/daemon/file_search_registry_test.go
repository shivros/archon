package daemon

import "testing"

func TestNewDaemonFileSearchRuntimeRegistryRegistersOpenCodeProviders(t *testing.T) {
	registry := newDaemonFileSearchRuntimeRegistry(nil, nil, nil)
	if _, ok := registry.Lookup("opencode"); !ok {
		t.Fatal("expected opencode file search provider")
	}
	if _, ok := registry.Lookup("kilocode"); !ok {
		t.Fatal("expected kilocode file search provider")
	}
	if _, ok := registry.Lookup("codex"); !ok {
		t.Fatal("expected codex file search provider")
	}
}
