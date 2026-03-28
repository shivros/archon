package providers

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReadmeSupportRowsMatchCurrentProviderSupportGrid(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("expected caller path")
	}
	readmePath := filepath.Join(filepath.Dir(filename), "..", "..", "README.md")
	content, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("read README: %v", err)
	}

	expectedRows := []string{
		approvalsSupportRow(t),
		interruptSupportRow(t),
		composeAutocompleteSupportRow(t),
		guidedWorkflowSupportRow(t),
	}
	for _, expectedRow := range expectedRows {
		if !strings.Contains(string(content), expectedRow) {
			t.Fatalf("expected README support row %q", expectedRow)
		}
	}
}

func approvalsSupportRow(t *testing.T) string {
	t.Helper()
	if !CapabilitiesFor("codex").SupportsApprovals {
		t.Fatal("expected codex approvals support")
	}
	if !CapabilitiesFor("claude").SupportsApprovals {
		t.Fatal("expected claude approvals support")
	}
	if !CapabilitiesFor("opencode").SupportsApprovals || !CapabilitiesFor("kilocode").SupportsApprovals {
		t.Fatal("expected OpenCode family approvals support")
	}
	return "| **Approvals** | Full | Partial | Partial |"
}

func interruptSupportRow(t *testing.T) string {
	t.Helper()
	if !CapabilitiesFor("codex").SupportsInterrupt {
		t.Fatal("expected codex interrupt support")
	}
	if !CapabilitiesFor("claude").SupportsInterrupt {
		t.Fatal("expected claude interrupt support")
	}
	if !CapabilitiesFor("opencode").SupportsInterrupt || !CapabilitiesFor("kilocode").SupportsInterrupt {
		t.Fatal("expected OpenCode family interrupt support")
	}
	return "| **Interrupt** | Full | Partial | Full |"
}

func composeAutocompleteSupportRow(t *testing.T) string {
	t.Helper()
	codex := readmeSupportLabel(CapabilitiesFor("codex").SupportsFileSearch)
	claude := readmeSupportLabel(CapabilitiesFor("claude").SupportsFileSearch)
	openCodeFamily := readmeSupportLabel(CapabilitiesFor("opencode").SupportsFileSearch && CapabilitiesFor("kilocode").SupportsFileSearch)
	return "| **Compose File Autocomplete (`@...`)** | " + codex + " | " + claude + " | " + openCodeFamily + " |"
}

func guidedWorkflowSupportRow(t *testing.T) string {
	t.Helper()
	if !CapabilitiesFor("codex").SupportsGuidedWorkflowDispatch {
		t.Fatal("expected codex guided workflow support")
	}
	if !CapabilitiesFor("claude").SupportsGuidedWorkflowDispatch {
		t.Fatal("expected claude guided workflow support")
	}
	if !CapabilitiesFor("opencode").SupportsGuidedWorkflowDispatch || !CapabilitiesFor("kilocode").SupportsGuidedWorkflowDispatch {
		t.Fatal("expected OpenCode family guided workflow support")
	}
	return "| **Guided Workflows** | Full | Full | Partial |"
}

func readmeSupportLabel(enabled bool) string {
	if enabled {
		return "Full"
	}
	return "-"
}
