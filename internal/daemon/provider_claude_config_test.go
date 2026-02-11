package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClaudeRunnerRunIncludePartialFromConfig(t *testing.T) {
	testBin := os.Args[0]
	wrapper := filepath.Join(t.TempDir(), "claude-wrapper.sh")
	wrapperScript := "#!/bin/sh\nexec \"" + testBin + "\" -test.run=TestHelperProcess -- \"$@\"\n"
	if err := os.WriteFile(wrapper, []byte(wrapperScript), 0o755); err != nil {
		t.Fatalf("WriteFile wrapper: %v", err)
	}

	tests := []struct {
		name           string
		includePartial bool
		expectFlag     bool
	}{
		{name: "enabled", includePartial: true, expectFlag: true},
		{name: "disabled", includePartial: false, expectFlag: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := filepath.Join(t.TempDir(), "home")
			t.Setenv("HOME", home)
			dataDir := filepath.Join(home, ".archon")
			if err := os.MkdirAll(dataDir, 0o700); err != nil {
				t.Fatalf("MkdirAll: %v", err)
			}
			uiCfg := "[providers.claude]\ninclude_partial = " + boolToTOML(tt.includePartial) + "\n"
			if err := os.WriteFile(filepath.Join(dataDir, "config.toml"), []byte(uiCfg), 0o600); err != nil {
				t.Fatalf("WriteFile config.toml: %v", err)
			}

			argsFile := filepath.Join(t.TempDir(), "claude-args.txt")
			runner := &claudeRunner{
				cmdName: wrapper,
				env:     []string{"GO_WANT_HELPER_PROCESS=1"},
			}
			if err := runner.run("args_file="+argsFile, nil); err != nil {
				t.Fatalf("runner.run: %v", err)
			}
			data, err := os.ReadFile(argsFile)
			if err != nil {
				t.Fatalf("ReadFile args: %v", err)
			}
			args := string(data)
			hasFlag := strings.Contains(args, "--include-partial-messages")
			if hasFlag != tt.expectFlag {
				t.Fatalf("include_partial=%v expected flag=%v, args=%q", tt.includePartial, tt.expectFlag, args)
			}
		})
	}
}

func boolToTOML(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
