package daemon

import (
	"os"
	"path/filepath"
	"testing"

	"control/internal/types"
)

func TestCodexOptionsFromConfig(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	dataDir := filepath.Join(home, ".archon")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	content := []byte(`
[providers.codex]
approval_policy = "on-request"
sandbox_policy = "workspace-write"
network_access = false
`)
	if err := os.WriteFile(filepath.Join(dataDir, "config.toml"), content, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	turn := codexTurnOptionsFromConfig()
	if turn["approvalPolicy"] != "on-request" {
		t.Fatalf("unexpected approval policy: %#v", turn["approvalPolicy"])
	}
	sandbox, ok := turn["sandboxPolicy"].(map[string]any)
	if !ok {
		t.Fatalf("expected sandboxPolicy map, got %#v", turn["sandboxPolicy"])
	}
	if sandbox["type"] != "workspaceWrite" {
		t.Fatalf("unexpected sandbox type: %#v", sandbox["type"])
	}
	if sandbox["networkAccess"] != false {
		t.Fatalf("unexpected network access: %#v", sandbox["networkAccess"])
	}

	thread := codexThreadOptionsFromConfig()
	if thread["approvalPolicy"] != "on-request" {
		t.Fatalf("unexpected thread approval policy: %#v", thread["approvalPolicy"])
	}
	if thread["sandbox"] != "workspace-write" {
		t.Fatalf("unexpected thread sandbox: %#v", thread["sandbox"])
	}
}

func TestCodexOptionsFromConfigEmpty(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	if got := codexTurnOptionsFromConfig(); got != nil {
		t.Fatalf("expected nil turn options, got %#v", got)
	}
	if got := codexThreadOptionsFromConfig(); got != nil {
		t.Fatalf("expected nil thread options, got %#v", got)
	}
}

func TestCodexOptionsRuntimeOverridesConfig(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	dataDir := filepath.Join(home, ".archon")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	content := []byte(`
[providers.codex]
approval_policy = "on-request"
sandbox_policy = "workspace-write"
`)
	if err := os.WriteFile(filepath.Join(dataDir, "config.toml"), content, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	runtime := &types.SessionRuntimeOptions{
		Access:    types.AccessFull,
		Reasoning: types.ReasoningExtraHigh,
	}
	turn := codexTurnOptions(runtime)
	if turn["approvalPolicy"] != "never" {
		t.Fatalf("expected runtime approval policy override, got %#v", turn["approvalPolicy"])
	}
	policy, ok := turn["sandboxPolicy"].(map[string]any)
	if !ok || policy["type"] != "dangerFullAccess" {
		t.Fatalf("unexpected runtime sandbox policy: %#v", turn["sandboxPolicy"])
	}
	if turn["reasoningEffort"] != "high" {
		t.Fatalf("expected extra_high reasoning to map to high, got %#v", turn["reasoningEffort"])
	}

	thread := codexThreadOptions(runtime)
	if thread["approvalPolicy"] != "never" {
		t.Fatalf("expected runtime thread approval policy override, got %#v", thread["approvalPolicy"])
	}
	if thread["sandbox"] != "danger-full-access" {
		t.Fatalf("expected runtime thread sandbox override, got %#v", thread["sandbox"])
	}
}

func TestCodexSandboxTypeConversions(t *testing.T) {
	tests := []struct {
		raw        string
		turnWant   string
		threadWant string
	}{
		{raw: "workspace-write", turnWant: "workspaceWrite", threadWant: "workspace-write"},
		{raw: "read-only", turnWant: "readOnly", threadWant: "read-only"},
		{raw: "danger-full-access", turnWant: "dangerFullAccess", threadWant: "danger-full-access"},
		{raw: "external-sandbox", turnWant: "externalSandbox", threadWant: "external-sandbox"},
		{raw: "custom", turnWant: "custom", threadWant: "custom"},
	}
	for _, tt := range tests {
		if got := codexSandboxTurnType(tt.raw); got != tt.turnWant {
			t.Fatalf("turn sandbox conversion mismatch: raw=%q got=%q want=%q", tt.raw, got, tt.turnWant)
		}
		if got := codexSandboxThreadType(tt.turnWant); got != tt.threadWant {
			t.Fatalf("thread sandbox conversion mismatch: raw=%q got=%q want=%q", tt.turnWant, got, tt.threadWant)
		}
	}
}

func TestShouldRetryWithoutModel(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "invalid params", err: errString("Invalid params: model"), want: true},
		{name: "unknown model", err: errString("unknown model"), want: true},
		{name: "unsupported model", err: errString("unsupported model"), want: true},
		{name: "unrecognized model", err: errString("unrecognized model"), want: true},
		{name: "other", err: errString("timeout"), want: false},
	}
	for _, tt := range tests {
		if got := shouldRetryWithoutModel(tt.err); got != tt.want {
			t.Fatalf("%s: expected %v got %v", tt.name, tt.want, got)
		}
	}
}

type errString string

func (e errString) Error() string {
	return string(e)
}

func TestCodexReasoningEffortMappings(t *testing.T) {
	tests := []struct {
		level types.ReasoningLevel
		want  string
	}{
		{level: types.ReasoningLow, want: "low"},
		{level: types.ReasoningMedium, want: "medium"},
		{level: types.ReasoningHigh, want: "high"},
		{level: types.ReasoningExtraHigh, want: "high"},
		{level: "unknown", want: ""},
	}
	for _, tt := range tests {
		if got := codexReasoningEffort(tt.level); got != tt.want {
			t.Fatalf("reasoning mapping mismatch: level=%q got=%q want=%q", tt.level, got, tt.want)
		}
	}
}

func TestCodexAccessPolicyMappings(t *testing.T) {
	tests := []struct {
		access        types.AccessLevel
		turnPolicy    string
		turnSandbox   string
		threadPolicy  string
		threadSandbox string
	}{
		{
			access:        types.AccessReadOnly,
			turnPolicy:    "on-request",
			turnSandbox:   "read-only",
			threadPolicy:  "on-request",
			threadSandbox: "read-only",
		},
		{
			access:        types.AccessOnRequest,
			turnPolicy:    "on-request",
			turnSandbox:   "workspace-write",
			threadPolicy:  "on-request",
			threadSandbox: "workspace-write",
		},
		{
			access:        types.AccessFull,
			turnPolicy:    "never",
			turnSandbox:   "danger-full-access",
			threadPolicy:  "never",
			threadSandbox: "danger-full-access",
		},
		{
			access:        "",
			turnPolicy:    "",
			turnSandbox:   "",
			threadPolicy:  "",
			threadSandbox: "",
		},
	}
	for _, tt := range tests {
		turnPolicy, turnSandbox := codexAccessToTurnPolicies(tt.access)
		if turnPolicy != tt.turnPolicy || turnSandbox != tt.turnSandbox {
			t.Fatalf("turn access mapping mismatch for %q: got=(%q,%q) want=(%q,%q)", tt.access, turnPolicy, turnSandbox, tt.turnPolicy, tt.turnSandbox)
		}
		threadPolicy, threadSandbox := codexAccessToThreadPolicies(tt.access)
		if threadPolicy != tt.threadPolicy || threadSandbox != tt.threadSandbox {
			t.Fatalf("thread access mapping mismatch for %q: got=(%q,%q) want=(%q,%q)", tt.access, threadPolicy, threadSandbox, tt.threadPolicy, tt.threadSandbox)
		}
	}
}
