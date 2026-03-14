package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDeprecatedWorkflowFieldsGuardScript(t *testing.T) {
	root := repoRootFromTestFile(t)

	t.Run("fails_on_non_test_go_source", func(t *testing.T) {
		scanRoot := t.TempDir()
		src := filepath.Join(scanRoot, "bad.go")
		if err := os.WriteFile(src, []byte("package p\n\nconst x = \"selected_policy_sensitivity\"\n"), 0o600); err != nil {
			t.Fatalf("write probe source: %v", err)
		}
		out, err := runDeprecatedFieldGuard(t, root, scanRoot)
		if err == nil {
			t.Fatalf("expected guard script to fail for deprecated field in non-test source, output=%q", out)
		}
		if !strings.Contains(out, "found reintroduced deprecated field selected_policy_sensitivity") {
			t.Fatalf("expected failure message in output, got %q", out)
		}
	})

	t.Run("ignores_test_go_source", func(t *testing.T) {
		scanRoot := t.TempDir()
		src := filepath.Join(scanRoot, "ignored_test.go")
		if err := os.WriteFile(src, []byte("package p\n\nconst x = \"selected_policy_sensitivity\"\n"), 0o600); err != nil {
			t.Fatalf("write probe source: %v", err)
		}
		out, err := runDeprecatedFieldGuard(t, root, scanRoot)
		if err != nil {
			t.Fatalf("expected guard script to ignore test files, err=%v output=%q", err, out)
		}
		if !strings.Contains(out, "check_deprecated_workflow_fields: ok") {
			t.Fatalf("expected success message in output, got %q", out)
		}
	})
}

func runDeprecatedFieldGuard(t *testing.T, root, scanRoot string) (string, error) {
	t.Helper()
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "check_deprecated_workflow_fields.sh"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "ARCHON_DEPRECATED_FIELD_SCAN_ROOTS="+scanRoot)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func repoRootFromTestFile(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("resolve caller path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
