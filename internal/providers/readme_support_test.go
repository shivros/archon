package providers

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReadmeComposeFileAutocompleteSupportRowMatchesCapabilities(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("expected caller path")
	}
	readmePath := filepath.Join(filepath.Dir(filename), "..", "..", "README.md")
	content, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("read README: %v", err)
	}

	expectedRow := composeAutocompleteSupportRow()
	if !strings.Contains(string(content), expectedRow) {
		t.Fatalf("expected README support row %q", expectedRow)
	}
}

func composeAutocompleteSupportRow() string {
	codex := readmeSupportLabel(CapabilitiesFor("codex").SupportsFileSearch)
	claude := readmeSupportLabel(CapabilitiesFor("claude").SupportsFileSearch)
	openCodeFamily := readmeSupportLabel(CapabilitiesFor("opencode").SupportsFileSearch && CapabilitiesFor("kilocode").SupportsFileSearch)
	return "| **Compose File Autocomplete (`@...`)** | " + codex + " | " + claude + " | " + openCodeFamily + " |"
}

func readmeSupportLabel(enabled bool) string {
	if enabled {
		return "Full"
	}
	return "-"
}
