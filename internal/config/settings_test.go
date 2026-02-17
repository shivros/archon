package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCoreConfigDefaults(t *testing.T) {
	t.Setenv("HOME", filepath.Join(t.TempDir(), "home"))
	cfg, err := LoadCoreConfig()
	if err != nil {
		t.Fatalf("LoadCoreConfig: %v", err)
	}
	if cfg.DaemonAddress() != "127.0.0.1:7777" {
		t.Fatalf("unexpected daemon address: %q", cfg.DaemonAddress())
	}
	if cfg.DaemonBaseURL() != "http://127.0.0.1:7777" {
		t.Fatalf("unexpected daemon base url: %q", cfg.DaemonBaseURL())
	}
}

func TestLoadCoreConfigFromTOML(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)

	dataDir := filepath.Join(home, ".archon")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	content := []byte(`
[daemon]
address = "http://127.0.0.1:9999/"

[logging]
level = "debug"

[debug]
stream_debug = true

[notifications]
enabled = true
triggers = ["turn.completed", "session.failed"]
methods = ["notify-send", "bell"]
script_commands = ["~/.archon/scripts/notify.sh"]
script_timeout_seconds = 20
dedupe_window_seconds = 8

[guided_workflows]
enabled = true
auto_start = true
checkpoint_style = "confidence-weighted"
mode = "guarded_autopilot"

[guided_workflows.policy]
confidence_threshold = 0.75
pause_threshold = 0.55
high_blast_radius_file_count = 30

[guided_workflows.policy.hard_gates]
ambiguity_blocker = true
sensitive_files = true
pre_commit_approval = true
failing_checks = true

[guided_workflows.policy.conditional_gates]
confidence_below_threshold = true
high_blast_radius = true
sensitive_files = false

[guided_workflows.rollout]
telemetry_enabled = false
max_active_runs = 5
automation_enabled = true
allow_quality_checks = true
allow_commit = true
require_commit_approval = false
max_retry_attempts = 4

[providers.codex]
command = "/usr/local/bin/codex"
default_model = "gpt-5.3-codex"
models = ["gpt-5.3-codex", "gpt-5.2-codex"]
approval_policy = "on-request"
sandbox_policy = "workspace-write"
network_access = false

[providers.claude]
command = "/usr/local/bin/claude"
default_model = "opus"
models = ["opus", "sonnet"]
include_partial = true

[providers.opencode]
command = "/usr/local/bin/opencode"
base_url = "http://127.0.0.1:4096"
token = "config-open"
token_env = "OPENCODE_TOKEN"
username = "archon"
timeout_seconds = 15

[providers.kilocode]
command = "/usr/local/bin/kilocode"
base_url = "http://127.0.0.1:4097"
token = "config-kilo"
token_env = "KILOCODE_TOKEN"
username = "archon-kilo"
timeout_seconds = 16

[providers.gemini]
command = "/usr/local/bin/gemini"
`)
	if err := os.WriteFile(filepath.Join(dataDir, "config.toml"), content, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := LoadCoreConfig()
	if err != nil {
		t.Fatalf("LoadCoreConfig: %v", err)
	}
	if cfg.DaemonAddress() != "127.0.0.1:9999" {
		t.Fatalf("unexpected daemon address: %q", cfg.DaemonAddress())
	}
	if cfg.DaemonBaseURL() != "http://127.0.0.1:9999" {
		t.Fatalf("unexpected daemon base url: %q", cfg.DaemonBaseURL())
	}
	if got := cfg.LogLevel(); got != "debug" {
		t.Fatalf("unexpected log level: %q", got)
	}
	if !cfg.StreamDebugEnabled() {
		t.Fatalf("expected stream_debug=true")
	}
	if !cfg.NotificationsEnabled() {
		t.Fatalf("expected notifications enabled=true")
	}
	if got := cfg.NotificationScriptTimeoutSeconds(); got != 20 {
		t.Fatalf("unexpected notification script timeout: %d", got)
	}
	if got := cfg.NotificationDedupeWindowSeconds(); got != 8 {
		t.Fatalf("unexpected notification dedupe window: %d", got)
	}
	if got := cfg.NotificationTriggers(); len(got) != 2 || got[0] != "turn.completed" {
		t.Fatalf("unexpected notification triggers: %#v", got)
	}
	if got := cfg.NotificationMethods(); len(got) != 2 || got[0] != "notify-send" {
		t.Fatalf("unexpected notification methods: %#v", got)
	}
	if got := cfg.NotificationScriptCommands(); len(got) != 1 || got[0] != "~/.archon/scripts/notify.sh" {
		t.Fatalf("unexpected notification script commands: %#v", got)
	}
	if !cfg.GuidedWorkflowsEnabled() {
		t.Fatalf("expected guided workflows enabled=true")
	}
	if !cfg.GuidedWorkflowsAutoStart() {
		t.Fatalf("expected guided workflows auto_start=true")
	}
	if got := cfg.GuidedWorkflowsCheckpointStyle(); got != "confidence_weighted" {
		t.Fatalf("unexpected guided workflows checkpoint style: %q", got)
	}
	if got := cfg.GuidedWorkflowsMode(); got != "guarded_autopilot" {
		t.Fatalf("unexpected guided workflows mode: %q", got)
	}
	if got := cfg.GuidedWorkflowsPolicyConfidenceThreshold(); got != 0.75 {
		t.Fatalf("unexpected guided workflows confidence threshold: %v", got)
	}
	if got := cfg.GuidedWorkflowsPolicyPauseThreshold(); got != 0.55 {
		t.Fatalf("unexpected guided workflows pause threshold: %v", got)
	}
	if got := cfg.GuidedWorkflowsPolicyHighBlastRadiusFileCount(); got != 30 {
		t.Fatalf("unexpected guided workflows high blast radius file count: %d", got)
	}
	if !cfg.GuidedWorkflowsPolicyHardGatePreCommitApproval() {
		t.Fatalf("expected guided workflows hard gate pre-commit approval=true")
	}
	if !cfg.GuidedWorkflowsPolicyConditionalGateConfidenceBelowThreshold() {
		t.Fatalf("expected guided workflows conditional confidence gate=true")
	}
	if cfg.GuidedWorkflowsRolloutTelemetryEnabled() {
		t.Fatalf("expected guided workflow rollout telemetry enabled=false from config")
	}
	if got := cfg.GuidedWorkflowsRolloutMaxActiveRuns(); got != 5 {
		t.Fatalf("unexpected guided workflows rollout max active runs: %d", got)
	}
	if !cfg.GuidedWorkflowsRolloutAutomationEnabled() {
		t.Fatalf("expected guided workflows rollout automation enabled=true")
	}
	if !cfg.GuidedWorkflowsRolloutAllowQualityChecks() {
		t.Fatalf("expected guided workflows rollout allow_quality_checks=true")
	}
	if !cfg.GuidedWorkflowsRolloutAllowCommit() {
		t.Fatalf("expected guided workflows rollout allow_commit=true")
	}
	if cfg.GuidedWorkflowsRolloutRequireCommitApproval() {
		t.Fatalf("expected guided workflows rollout require_commit_approval=false")
	}
	if got := cfg.GuidedWorkflowsRolloutMaxRetryAttempts(); got != 4 {
		t.Fatalf("unexpected guided workflows rollout max retry attempts: %d", got)
	}
	if got := cfg.ProviderCommand("codex"); got != "/usr/local/bin/codex" {
		t.Fatalf("unexpected codex command: %q", got)
	}
	if got := cfg.ProviderCommand("claude"); got != "/usr/local/bin/claude" {
		t.Fatalf("unexpected claude command: %q", got)
	}
	if got := cfg.ProviderCommand("opencode"); got != "/usr/local/bin/opencode" {
		t.Fatalf("unexpected opencode command: %q", got)
	}
	if got := cfg.ProviderCommand("kilocode"); got != "/usr/local/bin/kilocode" {
		t.Fatalf("unexpected kilocode command: %q", got)
	}
	if got := cfg.ProviderCommand("gemini"); got != "/usr/local/bin/gemini" {
		t.Fatalf("unexpected gemini command: %q", got)
	}
	if got := cfg.OpenCodeBaseURL("opencode"); got != "http://127.0.0.1:4096" {
		t.Fatalf("unexpected opencode base url: %q", got)
	}
	if got := cfg.OpenCodeBaseURL("kilocode"); got != "http://127.0.0.1:4097" {
		t.Fatalf("unexpected kilocode base url: %q", got)
	}
	if got := cfg.OpenCodeToken("opencode"); got != "config-open" {
		t.Fatalf("unexpected opencode token: %q", got)
	}
	if got := cfg.OpenCodeToken("kilocode"); got != "config-kilo" {
		t.Fatalf("unexpected kilocode token: %q", got)
	}
	if got := cfg.OpenCodeTokenEnv("opencode"); got != "OPENCODE_TOKEN" {
		t.Fatalf("unexpected opencode token env: %q", got)
	}
	if got := cfg.OpenCodeTokenEnv("kilocode"); got != "KILOCODE_TOKEN" {
		t.Fatalf("unexpected kilocode token env: %q", got)
	}
	if got := cfg.OpenCodeUsername("opencode"); got != "archon" {
		t.Fatalf("unexpected opencode username: %q", got)
	}
	if got := cfg.OpenCodeUsername("kilocode"); got != "archon-kilo" {
		t.Fatalf("unexpected kilocode username: %q", got)
	}
	if got := cfg.OpenCodeTimeoutSeconds("opencode"); got != 15 {
		t.Fatalf("unexpected opencode timeout: %d", got)
	}
	if got := cfg.OpenCodeTimeoutSeconds("kilocode"); got != 16 {
		t.Fatalf("unexpected kilocode timeout: %d", got)
	}
	if got := cfg.CodexDefaultModel(); got != "gpt-5.3-codex" {
		t.Fatalf("unexpected codex default model: %q", got)
	}
	if got := cfg.ClaudeDefaultModel(); got != "opus" {
		t.Fatalf("unexpected claude default model: %q", got)
	}
	if got := cfg.CodexModels(); len(got) != 2 || got[0] != "gpt-5.3-codex" {
		t.Fatalf("unexpected codex models: %#v", got)
	}
	if got := cfg.ClaudeModels(); len(got) != 2 || got[0] != "opus" {
		t.Fatalf("unexpected claude models: %#v", got)
	}
	if !cfg.ClaudeIncludePartial() {
		t.Fatalf("expected include_partial=true")
	}
	if got := cfg.CodexApprovalPolicy(); got != "on-request" {
		t.Fatalf("unexpected codex approval policy: %q", got)
	}
	if got := cfg.CodexSandboxPolicy(); got != "workspace-write" {
		t.Fatalf("unexpected codex sandbox policy: %q", got)
	}
	if value, ok := cfg.CodexNetworkAccess(); !ok || value {
		t.Fatalf("unexpected codex network_access: value=%v ok=%v", value, ok)
	}
}

func TestUIConfigResolveKeybindingsPath(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)

	cfg := UIConfig{}
	path, err := cfg.ResolveKeybindingsPath()
	if err != nil {
		t.Fatalf("ResolveKeybindingsPath default: %v", err)
	}
	if want := filepath.Join(home, ".archon", "keybindings.json"); path != want {
		t.Fatalf("unexpected default path: got=%q want=%q", path, want)
	}

	cfg.Keybindings.Path = "ui/keys.json"
	path, err = cfg.ResolveKeybindingsPath()
	if err != nil {
		t.Fatalf("ResolveKeybindingsPath relative: %v", err)
	}
	if want := filepath.Join(home, ".archon", "ui", "keys.json"); path != want {
		t.Fatalf("unexpected relative path: got=%q want=%q", path, want)
	}
}

func TestCoreConfigProviderDefaults(t *testing.T) {
	t.Setenv("HOME", filepath.Join(t.TempDir(), "home"))
	cfg, err := LoadCoreConfig()
	if err != nil {
		t.Fatalf("LoadCoreConfig: %v", err)
	}
	if cfg.CodexDefaultModel() == "" || len(cfg.CodexModels()) == 0 {
		t.Fatalf("expected codex defaults")
	}
	if cfg.ClaudeDefaultModel() == "" || len(cfg.ClaudeModels()) == 0 {
		t.Fatalf("expected claude defaults")
	}
	if _, ok := cfg.CodexNetworkAccess(); ok {
		t.Fatalf("expected codex network access unset by default")
	}
	if got := cfg.LogLevel(); got != "info" {
		t.Fatalf("expected default log level info, got %q", got)
	}
	if cfg.StreamDebugEnabled() {
		t.Fatalf("expected stream debug disabled by default")
	}
	if !cfg.NotificationsEnabled() {
		t.Fatalf("expected notifications enabled by default")
	}
	if got := cfg.NotificationScriptTimeoutSeconds(); got != 10 {
		t.Fatalf("unexpected default notification script timeout: %d", got)
	}
	if got := cfg.NotificationDedupeWindowSeconds(); got != 5 {
		t.Fatalf("unexpected default notification dedupe window: %d", got)
	}
	if got := cfg.NotificationMethods(); len(got) == 0 || got[0] != "auto" {
		t.Fatalf("unexpected default notification methods: %#v", got)
	}
	if cfg.GuidedWorkflowsEnabled() {
		t.Fatalf("expected guided workflows disabled by default")
	}
	if cfg.GuidedWorkflowsAutoStart() {
		t.Fatalf("expected guided workflows auto start disabled by default")
	}
	if got := cfg.GuidedWorkflowsCheckpointStyle(); got != "confidence_weighted" {
		t.Fatalf("unexpected default guided workflows checkpoint style: %q", got)
	}
	if got := cfg.GuidedWorkflowsMode(); got != "guarded_autopilot" {
		t.Fatalf("unexpected default guided workflows mode: %q", got)
	}
	if got := cfg.GuidedWorkflowsPolicyConfidenceThreshold(); got != 0.70 {
		t.Fatalf("unexpected default guided workflows confidence threshold: %v", got)
	}
	if got := cfg.GuidedWorkflowsPolicyPauseThreshold(); got != 0.60 {
		t.Fatalf("unexpected default guided workflows pause threshold: %v", got)
	}
	if got := cfg.GuidedWorkflowsPolicyHighBlastRadiusFileCount(); got != 20 {
		t.Fatalf("unexpected default guided workflows high blast radius file count: %d", got)
	}
	if !cfg.GuidedWorkflowsPolicyHardGateAmbiguityBlocker() {
		t.Fatalf("expected default hard gate ambiguity blocker=true")
	}
	if cfg.GuidedWorkflowsPolicyHardGatePreCommitApproval() {
		t.Fatalf("expected default hard gate pre-commit approval=false")
	}
	if !cfg.GuidedWorkflowsPolicyConditionalGateHighBlastRadius() {
		t.Fatalf("expected default conditional high blast radius gate=true")
	}
	if !cfg.GuidedWorkflowsRolloutTelemetryEnabled() {
		t.Fatalf("expected guided workflow rollout telemetry enabled by default")
	}
	if got := cfg.GuidedWorkflowsRolloutMaxActiveRuns(); got != 3 {
		t.Fatalf("unexpected guided workflow rollout max active runs: %d", got)
	}
	if cfg.GuidedWorkflowsRolloutAutomationEnabled() {
		t.Fatalf("expected guided workflow rollout automation disabled by default")
	}
	if cfg.GuidedWorkflowsRolloutAllowQualityChecks() {
		t.Fatalf("expected guided workflow rollout quality checks disabled by default")
	}
	if cfg.GuidedWorkflowsRolloutAllowCommit() {
		t.Fatalf("expected guided workflow rollout commit disabled by default")
	}
	if !cfg.GuidedWorkflowsRolloutRequireCommitApproval() {
		t.Fatalf("expected guided workflow rollout commit approval required by default")
	}
	if got := cfg.GuidedWorkflowsRolloutMaxRetryAttempts(); got != 2 {
		t.Fatalf("unexpected guided workflow rollout max retry attempts: %d", got)
	}
	if got := cfg.OpenCodeUsername("opencode"); got != "opencode" {
		t.Fatalf("unexpected default opencode username: %q", got)
	}
	if got := cfg.OpenCodeUsername("kilocode"); got != "kilocode" {
		t.Fatalf("unexpected default kilocode username: %q", got)
	}
	if got := cfg.OpenCodeBaseURL("opencode"); got != "http://127.0.0.1:4096" {
		t.Fatalf("unexpected default opencode base url: %q", got)
	}
	if got := cfg.OpenCodeBaseURL("kilocode"); got != "http://127.0.0.1:4097" {
		t.Fatalf("unexpected default kilocode base url: %q", got)
	}
	if got := cfg.OpenCodeTimeoutSeconds("opencode"); got != 30 {
		t.Fatalf("unexpected default opencode timeout: %d", got)
	}
}

func TestLoadUIConfigDefaults(t *testing.T) {
	t.Setenv("HOME", filepath.Join(t.TempDir(), "home"))
	cfg, err := LoadUIConfig()
	if err != nil {
		t.Fatalf("LoadUIConfig: %v", err)
	}
	minHeight, maxHeight := cfg.SharedMultilineInputHeights()
	if minHeight != 3 || maxHeight != 8 {
		t.Fatalf("unexpected shared multiline input defaults: min=%d max=%d", minHeight, maxHeight)
	}
	if mode := cfg.ChatTimestampMode(); mode != "relative" {
		t.Fatalf("unexpected default chat timestamp mode: %q", mode)
	}
	if !cfg.SidebarExpandByDefault() {
		t.Fatalf("expected sidebar expand_by_default=true by default")
	}
	if !cfg.SidebarShowRecents() {
		t.Fatalf("expected sidebar show_recents=true by default")
	}
	path, err := cfg.ResolveKeybindingsPath()
	if err != nil {
		t.Fatalf("ResolveKeybindingsPath: %v", err)
	}
	if want := filepath.Join(os.Getenv("HOME"), ".archon", "keybindings.json"); path != want {
		t.Fatalf("unexpected keybindings path: got=%q want=%q", path, want)
	}
}

func TestLoadUIConfigFromTOML(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	dataDir := filepath.Join(home, ".archon")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	content := []byte("[keybindings]\npath = \"~/custom-keys.json\"\n\n[input]\nmultiline_min_height = 4\nmultiline_max_height = 10\n\n[chat]\ntimestamp_mode = \"iso\"\n")
	if err := os.WriteFile(filepath.Join(dataDir, "ui.toml"), content, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := LoadUIConfig()
	if err != nil {
		t.Fatalf("LoadUIConfig: %v", err)
	}
	path, err := cfg.ResolveKeybindingsPath()
	if err != nil {
		t.Fatalf("ResolveKeybindingsPath: %v", err)
	}
	if want := filepath.Join(home, "custom-keys.json"); path != want {
		t.Fatalf("unexpected keybindings path: got=%q want=%q", path, want)
	}
	minHeight, maxHeight := cfg.SharedMultilineInputHeights()
	if minHeight != 4 || maxHeight != 10 {
		t.Fatalf("unexpected shared multiline input values: min=%d max=%d", minHeight, maxHeight)
	}
	if mode := cfg.ChatTimestampMode(); mode != "iso" {
		t.Fatalf("unexpected chat timestamp mode: %q", mode)
	}
	if !cfg.SidebarExpandByDefault() {
		t.Fatalf("expected sidebar expand_by_default to remain true when omitted")
	}
	if !cfg.SidebarShowRecents() {
		t.Fatalf("expected sidebar show_recents to remain true when omitted")
	}
}

func TestLoadUIConfigSidebarExpandByDefaultOverride(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	dataDir := filepath.Join(home, ".archon")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	content := []byte("[sidebar]\nexpand_by_default = false\n")
	if err := os.WriteFile(filepath.Join(dataDir, "ui.toml"), content, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := LoadUIConfig()
	if err != nil {
		t.Fatalf("LoadUIConfig: %v", err)
	}
	if cfg.SidebarExpandByDefault() {
		t.Fatalf("expected sidebar expand_by_default=false from config")
	}
}

func TestLoadUIConfigSidebarShowRecentsOverride(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	dataDir := filepath.Join(home, ".archon")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	content := []byte("[sidebar]\nshow_recents = false\n")
	if err := os.WriteFile(filepath.Join(dataDir, "ui.toml"), content, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := LoadUIConfig()
	if err != nil {
		t.Fatalf("LoadUIConfig: %v", err)
	}
	if cfg.SidebarShowRecents() {
		t.Fatalf("expected sidebar show_recents=false from config")
	}
}

func TestLoadUIConfigInvalidTOML(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	dataDir := filepath.Join(home, ".archon")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "ui.toml"), []byte("[keybindings\npath = \"x\""), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := LoadUIConfig(); err == nil {
		t.Fatalf("expected invalid TOML error")
	}
}

func TestLoadCoreConfigInvalidTOML(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	dataDir := filepath.Join(home, ".archon")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "config.toml"), []byte("[daemon\naddress = \"bad\""), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := LoadCoreConfig(); err == nil {
		t.Fatalf("expected invalid TOML error")
	}
}

func TestCoreConfigAccessorsNormalizeValues(t *testing.T) {
	networkAccess := true
	cfg := CoreConfig{
		Daemon: CoreDaemonConfig{
			Address: " https://127.0.0.1:9001/ ",
		},
		Logging: CoreLoggingConfig{
			Level: " debug ",
		},
		Debug: CoreDebugConfig{
			StreamDebug: true,
		},
		GuidedWorkflows: CoreGuidedWorkflowsConfig{
			Enabled:         boolPtr(true),
			AutoStart:       boolPtr(true),
			CheckpointStyle: " confidence-weighted ",
			Mode:            " guarded-autopilot ",
			Policy: CoreGuidedWorkflowsPolicyConfig{
				ConfidenceThreshold:      0.80,
				PauseThreshold:           0.65,
				HighBlastRadiusFileCount: 40,
				HardGates: CoreGuidedWorkflowsPolicyGatesConfig{
					AmbiguityBlocker:         boolPtr(false),
					ConfidenceBelowThreshold: boolPtr(true),
					HighBlastRadius:          boolPtr(true),
					SensitiveFiles:           boolPtr(false),
					PreCommitApproval:        boolPtr(true),
					FailingChecks:            boolPtr(false),
				},
				ConditionalGates: CoreGuidedWorkflowsPolicyGatesConfig{
					AmbiguityBlocker:         boolPtr(false),
					ConfidenceBelowThreshold: boolPtr(true),
					HighBlastRadius:          boolPtr(false),
					SensitiveFiles:           boolPtr(true),
					PreCommitApproval:        boolPtr(false),
					FailingChecks:            boolPtr(true),
				},
			},
			Rollout: CoreGuidedWorkflowsRolloutConfig{
				TelemetryEnabled:      boolPtr(false),
				MaxActiveRuns:         9,
				AutomationEnabled:     boolPtr(true),
				AllowQualityChecks:    boolPtr(true),
				AllowCommit:           boolPtr(true),
				RequireCommitApproval: boolPtr(false),
				MaxRetryAttempts:      99,
			},
		},
		Providers: CoreProvidersConfig{
			Codex: CoreCodexProviderConfig{
				Command:        " codex-bin ",
				DefaultModel:   " gpt-5.2-codex ",
				Models:         []string{" gpt-5.2-codex ", "", "gpt-5.2-codex", "gpt-5.3-codex"},
				ApprovalPolicy: " on-request ",
				SandboxPolicy:  " workspace-write ",
				NetworkAccess:  &networkAccess,
			},
			Claude: CoreClaudeProviderConfig{
				Command:      " claude-bin ",
				DefaultModel: " opus ",
				Models:       []string{" opus ", "sonnet", "opus"},
			},
			OpenCode: CoreOpenCodeProviderConfig{
				Command:        " opencode ",
				BaseURL:        " http://127.0.0.1:4096/ ",
				Token:          " config-open ",
				TokenEnv:       " OPENCODE_TOKEN ",
				Username:       " opencode-user ",
				TimeoutSeconds: 33,
			},
			KiloCode: CoreOpenCodeProviderConfig{
				Command:        " kilocode ",
				BaseURL:        " http://127.0.0.1:4097/ ",
				Token:          " config-kilo ",
				TokenEnv:       " KILOCODE_TOKEN ",
				Username:       " kilocode-user ",
				TimeoutSeconds: 34,
			},
			Gemini: CoreCommandProviderConfig{Command: " gemini "},
		},
	}

	if got := cfg.DaemonAddress(); got != "127.0.0.1:9001" {
		t.Fatalf("unexpected daemon address: %q", got)
	}
	if got := cfg.DaemonBaseURL(); got != "http://127.0.0.1:9001" {
		t.Fatalf("unexpected daemon base url: %q", got)
	}
	if got := cfg.LogLevel(); got != "debug" {
		t.Fatalf("unexpected log level: %q", got)
	}
	if !cfg.StreamDebugEnabled() {
		t.Fatalf("expected stream debug to be enabled")
	}
	if got := cfg.ProviderCommand("  CODEX "); got != "codex-bin" {
		t.Fatalf("unexpected codex command: %q", got)
	}
	if got := cfg.ProviderCommand("claude"); got != "claude-bin" {
		t.Fatalf("unexpected claude command: %q", got)
	}
	if got := cfg.ProviderCommand("opencode"); got != "opencode" {
		t.Fatalf("unexpected opencode command: %q", got)
	}
	if got := cfg.ProviderCommand("kilocode"); got != "kilocode" {
		t.Fatalf("unexpected kilocode command: %q", got)
	}
	if got := cfg.ProviderCommand("gemini"); got != "gemini" {
		t.Fatalf("unexpected gemini command: %q", got)
	}
	if got := cfg.OpenCodeBaseURL("opencode"); got != "http://127.0.0.1:4096/" {
		t.Fatalf("unexpected opencode base url: %q", got)
	}
	if got := cfg.OpenCodeBaseURL("kilocode"); got != "http://127.0.0.1:4097/" {
		t.Fatalf("unexpected kilocode base url: %q", got)
	}
	if got := cfg.OpenCodeToken("opencode"); got != "config-open" {
		t.Fatalf("unexpected opencode token: %q", got)
	}
	if got := cfg.OpenCodeToken("kilocode"); got != "config-kilo" {
		t.Fatalf("unexpected kilocode token: %q", got)
	}
	if got := cfg.OpenCodeTokenEnv("opencode"); got != "OPENCODE_TOKEN" {
		t.Fatalf("unexpected opencode token env: %q", got)
	}
	if got := cfg.OpenCodeTokenEnv("kilocode"); got != "KILOCODE_TOKEN" {
		t.Fatalf("unexpected kilocode token env: %q", got)
	}
	if got := cfg.OpenCodeUsername("opencode"); got != "opencode-user" {
		t.Fatalf("unexpected opencode username: %q", got)
	}
	if got := cfg.OpenCodeUsername("kilocode"); got != "kilocode-user" {
		t.Fatalf("unexpected kilocode username: %q", got)
	}
	if got := cfg.OpenCodeTimeoutSeconds("opencode"); got != 33 {
		t.Fatalf("unexpected opencode timeout: %d", got)
	}
	if got := cfg.OpenCodeTimeoutSeconds("kilocode"); got != 34 {
		t.Fatalf("unexpected kilocode timeout: %d", got)
	}
	if got := cfg.ProviderCommand("unknown"); got != "" {
		t.Fatalf("unexpected unknown provider command: %q", got)
	}
	if got := cfg.CodexDefaultModel(); got != "gpt-5.2-codex" {
		t.Fatalf("unexpected codex default model: %q", got)
	}
	if got := cfg.ClaudeDefaultModel(); got != "opus" {
		t.Fatalf("unexpected claude default model: %q", got)
	}
	if got := cfg.CodexModels(); len(got) != 2 || got[0] != "gpt-5.2-codex" || got[1] != "gpt-5.3-codex" {
		t.Fatalf("unexpected codex models: %#v", got)
	}
	if got := cfg.ClaudeModels(); len(got) != 2 || got[0] != "opus" || got[1] != "sonnet" {
		t.Fatalf("unexpected claude models: %#v", got)
	}
	if got := cfg.CodexApprovalPolicy(); got != "on-request" {
		t.Fatalf("unexpected approval policy: %q", got)
	}
	if got := cfg.CodexSandboxPolicy(); got != "workspace-write" {
		t.Fatalf("unexpected sandbox policy: %q", got)
	}
	if got, ok := cfg.CodexNetworkAccess(); !ok || !got {
		t.Fatalf("expected network access true, got value=%v ok=%v", got, ok)
	}
	if !cfg.GuidedWorkflowsEnabled() {
		t.Fatalf("expected guided workflows enabled")
	}
	if !cfg.GuidedWorkflowsAutoStart() {
		t.Fatalf("expected guided workflows auto_start enabled")
	}
	if got := cfg.GuidedWorkflowsCheckpointStyle(); got != "confidence_weighted" {
		t.Fatalf("unexpected guided workflows checkpoint style: %q", got)
	}
	if got := cfg.GuidedWorkflowsMode(); got != "guarded_autopilot" {
		t.Fatalf("unexpected guided workflows mode: %q", got)
	}
	if got := cfg.GuidedWorkflowsPolicyConfidenceThreshold(); got != 0.80 {
		t.Fatalf("unexpected guided workflows confidence threshold: %v", got)
	}
	if got := cfg.GuidedWorkflowsPolicyPauseThreshold(); got != 0.65 {
		t.Fatalf("unexpected guided workflows pause threshold: %v", got)
	}
	if got := cfg.GuidedWorkflowsPolicyHighBlastRadiusFileCount(); got != 40 {
		t.Fatalf("unexpected guided workflows high blast radius file count: %d", got)
	}
	if cfg.GuidedWorkflowsPolicyHardGateAmbiguityBlocker() {
		t.Fatalf("expected hard gate ambiguity blocker=false")
	}
	if !cfg.GuidedWorkflowsPolicyHardGatePreCommitApproval() {
		t.Fatalf("expected hard gate pre-commit approval=true")
	}
	if cfg.GuidedWorkflowsPolicyConditionalGateHighBlastRadius() {
		t.Fatalf("expected conditional high blast radius=false")
	}
	if cfg.GuidedWorkflowsRolloutTelemetryEnabled() {
		t.Fatalf("expected rollout telemetry enabled=false")
	}
	if got := cfg.GuidedWorkflowsRolloutMaxActiveRuns(); got != 9 {
		t.Fatalf("unexpected rollout max active runs: %d", got)
	}
	if !cfg.GuidedWorkflowsRolloutAutomationEnabled() {
		t.Fatalf("expected rollout automation enabled=true")
	}
	if !cfg.GuidedWorkflowsRolloutAllowQualityChecks() {
		t.Fatalf("expected rollout allow quality checks=true")
	}
	if !cfg.GuidedWorkflowsRolloutAllowCommit() {
		t.Fatalf("expected rollout allow commit=true")
	}
	if cfg.GuidedWorkflowsRolloutRequireCommitApproval() {
		t.Fatalf("expected rollout require commit approval=false")
	}
	if got := cfg.GuidedWorkflowsRolloutMaxRetryAttempts(); got != 5 {
		t.Fatalf("expected rollout max retry attempts clamped to 5, got %d", got)
	}
}

func TestUIConfigResolveKeybindingsAbsolutePath(t *testing.T) {
	absolute := filepath.Join(t.TempDir(), "keys.json")
	cfg := UIConfig{
		Keybindings: UIKeybindingsConfig{
			Path: absolute,
		},
	}
	path, err := cfg.ResolveKeybindingsPath()
	if err != nil {
		t.Fatalf("ResolveKeybindingsPath: %v", err)
	}
	if path != absolute {
		t.Fatalf("expected absolute path to remain unchanged, got %q", path)
	}
}

func TestUIConfigSharedMultilineInputHeightsClampsInvalidValues(t *testing.T) {
	cfg := UIConfig{
		Input: UIInputConfig{
			MultilineMinHeight: -3,
			MultilineMaxHeight: 2,
		},
	}
	minHeight, maxHeight := cfg.SharedMultilineInputHeights()
	if minHeight != 3 || maxHeight != 3 {
		t.Fatalf("unexpected clamped multiline input heights: min=%d max=%d", minHeight, maxHeight)
	}
}

func TestUIConfigChatTimestampModeNormalizesInvalidValue(t *testing.T) {
	cfg := UIConfig{
		Chat: UIChatConfig{
			TimestampMode: " weird ",
		},
	}
	if mode := cfg.ChatTimestampMode(); mode != "relative" {
		t.Fatalf("expected invalid mode to normalize to relative, got %q", mode)
	}
	cfg.Chat.TimestampMode = " ISO "
	if mode := cfg.ChatTimestampMode(); mode != "iso" {
		t.Fatalf("expected ISO mode to normalize, got %q", mode)
	}
}

func TestReadTOMLValidationAndEmptyFile(t *testing.T) {
	var cfg CoreConfig
	if err := readTOML("", &cfg); err == nil {
		t.Fatalf("expected path validation error")
	}

	path := filepath.Join(t.TempDir(), "empty.toml")
	if err := os.WriteFile(path, []byte(" \n "), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := readTOML(path, &cfg); err != nil {
		t.Fatalf("expected blank file to load as default, got %v", err)
	}
}

func TestResolveConfigPathValidation(t *testing.T) {
	if _, err := resolveConfigPath("   "); err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestCoreConfigLogLevelDefaultsWhenBlank(t *testing.T) {
	cfg := CoreConfig{
		Logging: CoreLoggingConfig{
			Level: "   ",
		},
	}
	if got := cfg.LogLevel(); got != "info" {
		t.Fatalf("expected info default, got %q", got)
	}
}

func TestGuidedWorkflowsConfigFallbacks(t *testing.T) {
	cfg := CoreConfig{
		GuidedWorkflows: CoreGuidedWorkflowsConfig{
			CheckpointStyle: "risk_based",
			Mode:            "manual",
			Policy: CoreGuidedWorkflowsPolicyConfig{
				ConfidenceThreshold:      0,
				PauseThreshold:           10,
				HighBlastRadiusFileCount: -1,
				HardGates: CoreGuidedWorkflowsPolicyGatesConfig{
					AmbiguityBlocker: nil,
				},
				ConditionalGates: CoreGuidedWorkflowsPolicyGatesConfig{
					HighBlastRadius: nil,
				},
			},
			Rollout: CoreGuidedWorkflowsRolloutConfig{
				TelemetryEnabled:      nil,
				MaxActiveRuns:         0,
				AutomationEnabled:     nil,
				AllowQualityChecks:    nil,
				AllowCommit:           nil,
				RequireCommitApproval: nil,
				MaxRetryAttempts:      0,
			},
		},
	}
	if cfg.GuidedWorkflowsEnabled() {
		t.Fatalf("expected guided workflows disabled when enabled unset")
	}
	if cfg.GuidedWorkflowsAutoStart() {
		t.Fatalf("expected guided workflows auto_start disabled when unset")
	}
	if got := cfg.GuidedWorkflowsCheckpointStyle(); got != "confidence_weighted" {
		t.Fatalf("unexpected checkpoint fallback: %q", got)
	}
	if got := cfg.GuidedWorkflowsMode(); got != "guarded_autopilot" {
		t.Fatalf("unexpected mode fallback: %q", got)
	}
	if got := cfg.GuidedWorkflowsPolicyConfidenceThreshold(); got != 0.70 {
		t.Fatalf("unexpected confidence threshold fallback: %v", got)
	}
	if got := cfg.GuidedWorkflowsPolicyPauseThreshold(); got != 0.60 {
		t.Fatalf("unexpected pause threshold fallback: %v", got)
	}
	if got := cfg.GuidedWorkflowsPolicyHighBlastRadiusFileCount(); got != 20 {
		t.Fatalf("unexpected high blast radius file count fallback: %d", got)
	}
	if !cfg.GuidedWorkflowsPolicyHardGateAmbiguityBlocker() {
		t.Fatalf("expected default hard gate ambiguity blocker=true")
	}
	if !cfg.GuidedWorkflowsPolicyConditionalGateHighBlastRadius() {
		t.Fatalf("expected default conditional gate high blast radius=true")
	}
	if !cfg.GuidedWorkflowsRolloutTelemetryEnabled() {
		t.Fatalf("expected default rollout telemetry enabled=true")
	}
	if got := cfg.GuidedWorkflowsRolloutMaxActiveRuns(); got != 3 {
		t.Fatalf("unexpected default rollout max active runs: %d", got)
	}
	if cfg.GuidedWorkflowsRolloutAutomationEnabled() {
		t.Fatalf("expected default rollout automation enabled=false")
	}
	if cfg.GuidedWorkflowsRolloutAllowQualityChecks() {
		t.Fatalf("expected default rollout allow quality checks=false")
	}
	if cfg.GuidedWorkflowsRolloutAllowCommit() {
		t.Fatalf("expected default rollout allow commit=false")
	}
	if !cfg.GuidedWorkflowsRolloutRequireCommitApproval() {
		t.Fatalf("expected default rollout require commit approval=true")
	}
	if got := cfg.GuidedWorkflowsRolloutMaxRetryAttempts(); got != 2 {
		t.Fatalf("unexpected default rollout max retry attempts: %d", got)
	}
}
