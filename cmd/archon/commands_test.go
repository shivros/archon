package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	controlclient "control/internal/client"
	"control/internal/types"
)

func TestDaemonCommandKillFlag(t *testing.T) {
	var calls []string
	cmd := NewDaemonCommand(
		&bytes.Buffer{},
		func(background bool) error {
			calls = append(calls, "run")
			if background {
				calls = append(calls, "background")
			}
			return nil
		},
		func() error {
			calls = append(calls, "kill")
			return nil
		},
	)

	if err := cmd.Run([]string{"--kill"}); err != nil {
		t.Fatalf("expected kill run to succeed, got err=%v", err)
	}
	if strings.Join(calls, ",") != "kill" {
		t.Fatalf("unexpected call order: %v", calls)
	}
}

func TestStartCommandWritesSessionID(t *testing.T) {
	stdout := &bytes.Buffer{}
	fake := &fakeCommandClient{
		startSessionResp: &types.Session{ID: "session-123"},
	}
	cmd := NewStartCommand(stdout, &bytes.Buffer{}, fixedSessionFactory(fake))

	err := cmd.Run([]string{
		"--provider", "codex",
		"--cwd", "/tmp/project",
		"--cmd", "codex",
		"--title", "demo",
		"--tag", "one",
		"--tag", "two",
		"--env", "A=B",
		"--env", "C=D",
		"arg1",
		"arg2",
	})
	if err != nil {
		t.Fatalf("expected start to succeed, got err=%v", err)
	}
	if fake.ensureDaemonCalls != 1 {
		t.Fatalf("expected ensure daemon once, got %d", fake.ensureDaemonCalls)
	}
	if len(fake.startRequests) != 1 {
		t.Fatalf("expected one start request, got %d", len(fake.startRequests))
	}
	req := fake.startRequests[0]
	if req.Provider != "codex" || req.Cwd != "/tmp/project" || req.Cmd != "codex" {
		t.Fatalf("unexpected start request basics: %#v", req)
	}
	if len(req.Args) != 2 || req.Args[0] != "arg1" || req.Args[1] != "arg2" {
		t.Fatalf("unexpected args: %#v", req.Args)
	}
	if len(req.Tags) != 2 || req.Tags[0] != "one" || req.Tags[1] != "two" {
		t.Fatalf("unexpected tags: %#v", req.Tags)
	}
	if len(req.Env) != 2 || req.Env[0] != "A=B" || req.Env[1] != "C=D" {
		t.Fatalf("unexpected env: %#v", req.Env)
	}
	if got := stdout.String(); got != "session-123\n" {
		t.Fatalf("unexpected stdout: %q", got)
	}
}

func TestPSCommandPrintsSessions(t *testing.T) {
	stdout := &bytes.Buffer{}
	fake := &fakeCommandClient{
		sessionsResp: []*types.Session{
			{ID: "s1", Status: types.SessionStatusRunning, Provider: "codex", PID: 42, Title: "demo"},
		},
	}
	cmd := NewPSCommand(stdout, &bytes.Buffer{}, fixedSessionFactory(fake))

	if err := cmd.Run(nil); err != nil {
		t.Fatalf("expected ps to succeed, got err=%v", err)
	}
	if fake.ensureDaemonCalls != 1 {
		t.Fatalf("expected ensure daemon once, got %d", fake.ensureDaemonCalls)
	}
	if fake.listSessionsCalls != 1 {
		t.Fatalf("expected list sessions once, got %d", fake.listSessionsCalls)
	}
	out := stdout.String()
	if !strings.Contains(out, "ID") || !strings.Contains(out, "STATUS") || !strings.Contains(out, "PROVIDER") {
		t.Fatalf("expected header in output, got %q", out)
	}
	if !strings.Contains(out, "s1") || !strings.Contains(out, "demo") {
		t.Fatalf("expected session row in output, got %q", out)
	}
}

func TestTailCommandWritesItemsJSON(t *testing.T) {
	stdout := &bytes.Buffer{}
	fake := &fakeCommandClient{
		tailItemsResp: &controlclient.TailItemsResponse{
			Items: []map[string]any{{"type": "log", "text": "hello"}},
		},
	}
	cmd := NewTailCommand(stdout, &bytes.Buffer{}, fixedSessionFactory(fake))

	if err := cmd.Run([]string{"--lines", "50", "session-1"}); err != nil {
		t.Fatalf("expected tail to succeed, got err=%v", err)
	}
	if fake.ensureDaemonCalls != 1 {
		t.Fatalf("expected ensure daemon once, got %d", fake.ensureDaemonCalls)
	}
	if fake.tailItemsCalls != 1 || fake.tailItemsID != "session-1" || fake.tailItemsLines != 50 {
		t.Fatalf("unexpected tail call details: calls=%d id=%q lines=%d", fake.tailItemsCalls, fake.tailItemsID, fake.tailItemsLines)
	}
	var items []map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &items); err != nil {
		t.Fatalf("expected valid json output, got err=%v, raw=%q", err, stdout.String())
	}
	if len(items) != 1 {
		t.Fatalf("expected one output item, got %d", len(items))
	}
}

func TestUICommandEnsuresVersionAndRunsUI(t *testing.T) {
	fake := &fakeCommandClient{}
	logConfigured := 0

	cmd := NewUICommand(
		&bytes.Buffer{},
		fixedDaemonVersionFactory(fake),
		func() { logConfigured++ },
		"v-test",
	)

	if err := cmd.Run([]string{"--restart-daemon"}); err != nil {
		t.Fatalf("expected ui command to succeed, got err=%v", err)
	}
	if logConfigured != 1 {
		t.Fatalf("expected UI logging to be configured once, got %d", logConfigured)
	}
	if fake.ensureVersionCalls != 1 {
		t.Fatalf("expected ensure daemon version once, got %d", fake.ensureVersionCalls)
	}
	if fake.ensureDaemonCalls != 0 {
		t.Fatalf("expected ensure daemon to not be called, got %d", fake.ensureDaemonCalls)
	}
	if fake.ensureVersionExpected != "v-test" || !fake.ensureVersionRestart {
		t.Fatalf("unexpected ensure version args: expected=%q restart=%v", fake.ensureVersionExpected, fake.ensureVersionRestart)
	}
	if fake.runUICalls != 1 {
		t.Fatalf("expected ui runner once, got %d", fake.runUICalls)
	}
}

func TestUICommandIgnoresVersionMismatchWhenFlagSet(t *testing.T) {
	fake := &fakeCommandClient{}
	logConfigured := 0

	cmd := NewUICommand(
		&bytes.Buffer{},
		fixedDaemonVersionFactory(fake),
		func() { logConfigured++ },
		"v-test",
	)

	if err := cmd.Run([]string{"--ignore-daemon-mismatch"}); err != nil {
		t.Fatalf("expected ui command to succeed, got err=%v", err)
	}
	if logConfigured != 1 {
		t.Fatalf("expected UI logging to be configured once, got %d", logConfigured)
	}
	if fake.ensureDaemonCalls != 1 {
		t.Fatalf("expected ensure daemon once, got %d", fake.ensureDaemonCalls)
	}
	if fake.ensureVersionCalls != 0 {
		t.Fatalf("expected ensure daemon version to not be called, got %d", fake.ensureVersionCalls)
	}
	if fake.runUICalls != 1 {
		t.Fatalf("expected ui runner once, got %d", fake.runUICalls)
	}
}

func TestStartCommandRequiresProvider(t *testing.T) {
	cmd := NewStartCommand(&bytes.Buffer{}, &bytes.Buffer{}, fixedSessionFactory(&fakeCommandClient{}))
	err := cmd.Run(nil)
	if err == nil || !strings.Contains(err.Error(), "provider is required") {
		t.Fatalf("expected provider validation error, got %v", err)
	}
}

func TestLoginCommandPrintsFallbackAndPollsUntilApproved(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	fake := &fakeCommandClient{
		cloudLoginStartResp: &controlclient.CloudDeviceAuthorizationResponse{
			DeviceCode:              "device-1",
			UserCode:                "ABCD-EFGH",
			VerificationURI:         "https://archon.example/activate",
			VerificationURIComplete: "https://archon.example/activate?user_code=ABCD-EFGH",
			ExpiresIn:               600,
			Interval:                0,
		},
		cloudPollResponses: []*controlclient.CloudAuthPollResponse{
			{Status: "authorization_pending"},
			{Status: "approved", Auth: &controlclient.CloudAuthStatusResponse{
				Linked: true,
				User:   &controlclient.CloudLinkedUser{Email: "user@example.com"},
			}},
		},
	}
	var opened string
	cmd := NewLoginCommand(stdout, stderr, fixedCloudAuthFactory(fake), func(_ context.Context, target string) error {
		opened = target
		return nil
	})
	cmd.sleep = func(context.Context, time.Duration) error { return nil }

	if err := cmd.Run(nil); err != nil {
		t.Fatalf("expected login to succeed, got err=%v", err)
	}
	if fake.ensureDaemonCalls != 1 {
		t.Fatalf("expected ensure daemon once, got %d", fake.ensureDaemonCalls)
	}
	if fake.cloudLoginStartCalls != 1 {
		t.Fatalf("expected cloud login start once, got %d", fake.cloudLoginStartCalls)
	}
	if fake.cloudPollCalls != 2 {
		t.Fatalf("expected two poll attempts, got %d", fake.cloudPollCalls)
	}
	if opened != "https://archon.example/activate?user_code=ABCD-EFGH" {
		t.Fatalf("unexpected browser target: %q", opened)
	}
	out := stdout.String()
	if !strings.Contains(out, "https://archon.example/activate") || !strings.Contains(out, "ABCD-EFGH") {
		t.Fatalf("expected fallback instructions in output, got %q", out)
	}
	if !strings.Contains(out, "user@example.com") {
		t.Fatalf("expected linked user output, got %q", out)
	}
}

func TestLoginCommandContinuesWhenBrowserOpenFails(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	fake := &fakeCommandClient{
		cloudLoginStartResp: &controlclient.CloudDeviceAuthorizationResponse{
			DeviceCode:      "device-1",
			UserCode:        "ABCD-EFGH",
			VerificationURI: "https://archon.example/activate",
			ExpiresIn:       600,
			Interval:        0,
		},
		cloudPollResponses: []*controlclient.CloudAuthPollResponse{
			{Status: "approved", Auth: &controlclient.CloudAuthStatusResponse{Linked: true}},
		},
	}
	cmd := NewLoginCommand(stdout, stderr, fixedCloudAuthFactory(fake), func(context.Context, string) error {
		return errors.New("boom")
	})
	cmd.sleep = func(context.Context, time.Duration) error { return nil }

	if err := cmd.Run(nil); err != nil {
		t.Fatalf("expected login to succeed, got err=%v", err)
	}
	if !strings.Contains(stderr.String(), "browser open failed") {
		t.Fatalf("expected browser failure warning, got %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), "Code: ABCD-EFGH") {
		t.Fatalf("expected fallback code in output, got %q", stdout.String())
	}
}

func TestLoginCommandHonorsNoBrowserFlag(t *testing.T) {
	stdout := &bytes.Buffer{}
	fake := &fakeCommandClient{
		cloudLoginStartResp: &controlclient.CloudDeviceAuthorizationResponse{
			DeviceCode:      "device-1",
			UserCode:        "ABCD-EFGH",
			VerificationURI: "https://archon.example/activate",
			ExpiresIn:       600,
		},
		cloudPollResponses: []*controlclient.CloudAuthPollResponse{
			{Status: "approved", Auth: &controlclient.CloudAuthStatusResponse{Linked: true}},
		},
	}
	called := false
	cmd := NewLoginCommand(stdout, &bytes.Buffer{}, fixedCloudAuthFactory(fake), func(context.Context, string) error {
		called = true
		return nil
	})
	cmd.sleep = func(context.Context, time.Duration) error { return nil }

	if err := cmd.Run([]string{"--no-browser"}); err != nil {
		t.Fatalf("expected login to succeed, got err=%v", err)
	}
	if called {
		t.Fatalf("expected browser opener not to be called")
	}
}

func TestLoginCommandHandlesSlowDownAndUnexpectedStatus(t *testing.T) {
	t.Run("slow down", func(t *testing.T) {
		fake := &fakeCommandClient{
			cloudLoginStartResp: &controlclient.CloudDeviceAuthorizationResponse{
				DeviceCode:      "device-1",
				UserCode:        "ABCD-EFGH",
				VerificationURI: "https://archon.example/activate",
				ExpiresIn:       600,
				Interval:        1,
			},
			cloudPollResponses: []*controlclient.CloudAuthPollResponse{
				{Status: "slow_down"},
				{Status: "approved", Auth: &controlclient.CloudAuthStatusResponse{Linked: true}},
			},
		}
		var sleeps []time.Duration
		cmd := NewLoginCommand(&bytes.Buffer{}, &bytes.Buffer{}, fixedCloudAuthFactory(fake), nil)
		cmd.sleep = func(_ context.Context, d time.Duration) error {
			sleeps = append(sleeps, d)
			return nil
		}
		if err := cmd.Run(nil); err != nil {
			t.Fatalf("expected login to succeed, got err=%v", err)
		}
		if len(sleeps) == 0 || sleeps[0] != 6*time.Second {
			t.Fatalf("expected slow_down backoff to 6s, got %#v", sleeps)
		}
	})

	t.Run("unexpected status", func(t *testing.T) {
		fake := &fakeCommandClient{
			cloudLoginStartResp: &controlclient.CloudDeviceAuthorizationResponse{
				DeviceCode:      "device-1",
				UserCode:        "ABCD-EFGH",
				VerificationURI: "https://archon.example/activate",
				ExpiresIn:       600,
			},
			cloudPollResponses: []*controlclient.CloudAuthPollResponse{
				{Status: "mystery"},
			},
		}
		cmd := NewLoginCommand(&bytes.Buffer{}, &bytes.Buffer{}, fixedCloudAuthFactory(fake), nil)
		cmd.sleep = func(context.Context, time.Duration) error { return nil }
		if err := cmd.Run(nil); err == nil || !strings.Contains(err.Error(), "unexpected cloud login status") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestLoginCommandHandlesPollErrorAndSleepCancellation(t *testing.T) {
	t.Run("poll error", func(t *testing.T) {
		fake := &fakeCommandClient{
			cloudLoginStartResp: &controlclient.CloudDeviceAuthorizationResponse{
				DeviceCode:      "device-1",
				UserCode:        "ABCD-EFGH",
				VerificationURI: "https://archon.example/activate",
				ExpiresIn:       600,
			},
			cloudPollErrs: []error{errors.New("poll failed")},
		}
		cmd := NewLoginCommand(&bytes.Buffer{}, &bytes.Buffer{}, fixedCloudAuthFactory(fake), nil)
		if err := cmd.Run(nil); err == nil || !strings.Contains(err.Error(), "poll failed") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("sleep canceled", func(t *testing.T) {
		fake := &fakeCommandClient{
			cloudLoginStartResp: &controlclient.CloudDeviceAuthorizationResponse{
				DeviceCode:      "device-1",
				UserCode:        "ABCD-EFGH",
				VerificationURI: "https://archon.example/activate",
				ExpiresIn:       600,
			},
			cloudPollResponses: []*controlclient.CloudAuthPollResponse{
				{Status: "authorization_pending"},
			},
		}
		cmd := NewLoginCommand(&bytes.Buffer{}, &bytes.Buffer{}, fixedCloudAuthFactory(fake), nil)
		cmd.sleep = func(context.Context, time.Duration) error { return context.Canceled }
		if err := cmd.Run(nil); !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context canceled, got %v", err)
		}
	})
}

func TestWhoAmICommandPrintsLinkedStatus(t *testing.T) {
	stdout := &bytes.Buffer{}
	fake := &fakeCommandClient{
		cloudStatusResp: &controlclient.CloudAuthStatusResponse{
			Linked:       true,
			User:         &controlclient.CloudLinkedUser{DisplayName: "Shiv"},
			Installation: &controlclient.CloudInstallation{Name: "Archon Laptop"},
		},
	}
	cmd := NewWhoAmICommand(stdout, &bytes.Buffer{}, fixedCloudAuthFactory(fake))

	if err := cmd.Run(nil); err != nil {
		t.Fatalf("expected whoami to succeed, got err=%v", err)
	}
	if !strings.Contains(stdout.String(), "Shiv") || !strings.Contains(stdout.String(), "Archon Laptop") {
		t.Fatalf("unexpected whoami output: %q", stdout.String())
	}
}

func TestLogoutCommandCallsClient(t *testing.T) {
	stdout := &bytes.Buffer{}
	fake := &fakeCommandClient{}
	cmd := NewLogoutCommand(stdout, &bytes.Buffer{}, fixedCloudAuthFactory(fake))

	if err := cmd.Run(nil); err != nil {
		t.Fatalf("expected logout to succeed, got err=%v", err)
	}
	if fake.logoutCloudCalls != 1 {
		t.Fatalf("expected logout call once, got %d", fake.logoutCloudCalls)
	}
	if got := stdout.String(); got != "revoked remote token and cleared local cloud credentials\n" {
		t.Fatalf("unexpected logout output: %q", got)
	}
}

func TestLogoutCommandPrintsPartialLogoutResult(t *testing.T) {
	stdout := &bytes.Buffer{}
	fake := &fakeCommandClient{
		logoutCloudResp: &controlclient.CloudLogoutResponse{
			Status:       "unlinked_local_only",
			LocalCleared: true,
			Message:      "remote revoke failed; cleared local cloud credentials only",
		},
	}
	cmd := NewLogoutCommand(stdout, &bytes.Buffer{}, fixedCloudAuthFactory(fake))

	if err := cmd.Run(nil); err != nil {
		t.Fatalf("expected logout to succeed, got err=%v", err)
	}
	if !strings.Contains(stdout.String(), "remote revoke failed") {
		t.Fatalf("unexpected partial logout output: %q", stdout.String())
	}
}

func TestSleepContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sleepContext(ctx, time.Second); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
}

func TestConfigCommandPrintsEffectiveConfig(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	dataDir := filepath.Join(home, ".archon")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	core := []byte(`
[daemon]
address = "127.0.0.1:8899"

[logging]
level = "debug"

[debug]
stream_debug = true

[guided_workflows.defaults]
provider = "codex"
model = "gpt-5.3-codex"
access = "on_request"
reasoning = "high"
resolution_boundary = "high"

[providers.codex]
default_model = "gpt-5.3-codex"
models = ["gpt-5.3-codex"]
approval_policy = "on-request"
sandbox_policy = "workspace-write"
network_access = false
`)
	if err := os.WriteFile(filepath.Join(dataDir, "config.toml"), core, 0o600); err != nil {
		t.Fatalf("WriteFile config.toml: %v", err)
	}
	ui := []byte("[keybindings]\npath = \"custom-keybindings.json\"\n")
	if err := os.WriteFile(filepath.Join(dataDir, "ui.toml"), ui, 0o600); err != nil {
		t.Fatalf("WriteFile ui.toml: %v", err)
	}
	keybindings := []byte(`{"ui.toggleSidebar":"alt+b","ui.refresh":"F5"}`)
	if err := os.WriteFile(filepath.Join(dataDir, "custom-keybindings.json"), keybindings, 0o600); err != nil {
		t.Fatalf("WriteFile keybindings: %v", err)
	}
	workflowTemplates := []byte(`{
  "version": 1,
  "templates": [
    {
      "id": "custom_delivery",
      "name": "Custom Delivery",
      "description": "A custom guided workflow",
      "default_access_level": "on_request",
      "phases": [
        {
          "id": "phase_1",
          "name": "Plan",
          "steps": [
            {
              "id": "step_1",
              "name": "Draft plan",
              "prompt": "Draft a plan."
            }
          ]
        }
      ]
    }
  ]
}`)
	if err := os.WriteFile(filepath.Join(dataDir, "workflow_templates.json"), workflowTemplates, 0o600); err != nil {
		t.Fatalf("WriteFile workflow_templates.json: %v", err)
	}

	stdout := &bytes.Buffer{}
	cmd := NewConfigCommand(stdout, &bytes.Buffer{})
	if err := cmd.Run(nil); err != nil {
		t.Fatalf("config command failed: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json: %v raw=%q", err, stdout.String())
	}
	daemon, _ := payload["daemon"].(map[string]any)
	if daemon["address"] != "127.0.0.1:8899" {
		t.Fatalf("unexpected daemon address: %#v", daemon["address"])
	}
	if daemon["base_url"] != "http://127.0.0.1:8899" {
		t.Fatalf("unexpected daemon base_url: %#v", daemon["base_url"])
	}
	loggingCfg, _ := payload["logging"].(map[string]any)
	if loggingCfg["level"] != "debug" {
		t.Fatalf("unexpected logging level: %#v", loggingCfg["level"])
	}
	debugCfg, _ := payload["debug"].(map[string]any)
	if debugCfg["stream_debug"] != true {
		t.Fatalf("unexpected stream_debug: %#v", debugCfg["stream_debug"])
	}
	notificationsCfg, _ := payload["notifications"].(map[string]any)
	if notificationsCfg["enabled"] != true {
		t.Fatalf("unexpected notifications enabled: %#v", notificationsCfg["enabled"])
	}
	guidedCfg, _ := payload["guided_workflows"].(map[string]any)
	if guidedCfg["enabled"] != false {
		t.Fatalf("unexpected guided workflows enabled: %#v", guidedCfg["enabled"])
	}
	if guidedCfg["auto_start"] != false {
		t.Fatalf("unexpected guided workflows auto_start: %#v", guidedCfg["auto_start"])
	}
	if guidedCfg["checkpoint_style"] != "confidence_weighted" {
		t.Fatalf("unexpected guided workflows checkpoint_style: %#v", guidedCfg["checkpoint_style"])
	}
	if guidedCfg["mode"] != "guarded_autopilot" {
		t.Fatalf("unexpected guided workflows mode: %#v", guidedCfg["mode"])
	}
	defaultsCfg, _ := guidedCfg["defaults"].(map[string]any)
	if defaultsCfg["provider"] != "codex" {
		t.Fatalf("unexpected guided workflows defaults provider: %#v", defaultsCfg["provider"])
	}
	if defaultsCfg["model"] != "gpt-5.3-codex" {
		t.Fatalf("unexpected guided workflows defaults model: %#v", defaultsCfg["model"])
	}
	if defaultsCfg["access"] != "on_request" {
		t.Fatalf("unexpected guided workflows defaults access: %#v", defaultsCfg["access"])
	}
	if defaultsCfg["reasoning"] != "high" {
		t.Fatalf("unexpected guided workflows defaults reasoning: %#v", defaultsCfg["reasoning"])
	}
	if defaultsCfg["resolution_boundary"] != "high" {
		t.Fatalf("unexpected guided workflows defaults resolution boundary: %#v", defaultsCfg["resolution_boundary"])
	}
	policyCfg, _ := guidedCfg["policy"].(map[string]any)
	if policyCfg["confidence_threshold"] != 0.7 {
		t.Fatalf("unexpected guided workflows confidence_threshold: %#v", policyCfg["confidence_threshold"])
	}
	if policyCfg["pause_threshold"] != 0.6 {
		t.Fatalf("unexpected guided workflows pause_threshold: %#v", policyCfg["pause_threshold"])
	}
	if policyCfg["high_blast_radius_file_count"] != float64(20) {
		t.Fatalf("unexpected guided workflows high_blast_radius_file_count: %#v", policyCfg["high_blast_radius_file_count"])
	}
	hardGates, _ := policyCfg["hard_gates"].(map[string]any)
	if hardGates["ambiguity_blocker"] != true || hardGates["sensitive_files"] != true || hardGates["failing_checks"] != true {
		t.Fatalf("unexpected guided workflows hard_gates: %#v", hardGates)
	}
	rolloutCfg, _ := guidedCfg["rollout"].(map[string]any)
	if rolloutCfg["telemetry_enabled"] != true {
		t.Fatalf("unexpected rollout telemetry_enabled: %#v", rolloutCfg["telemetry_enabled"])
	}
	if rolloutCfg["max_active_runs"] != float64(3) {
		t.Fatalf("unexpected rollout max_active_runs: %#v", rolloutCfg["max_active_runs"])
	}
	if rolloutCfg["automation_enabled"] != false {
		t.Fatalf("unexpected rollout automation_enabled: %#v", rolloutCfg["automation_enabled"])
	}
	if rolloutCfg["allow_quality_checks"] != false {
		t.Fatalf("unexpected rollout allow_quality_checks: %#v", rolloutCfg["allow_quality_checks"])
	}
	if rolloutCfg["allow_commit"] != false {
		t.Fatalf("unexpected rollout allow_commit: %#v", rolloutCfg["allow_commit"])
	}
	if rolloutCfg["require_commit_approval"] != true {
		t.Fatalf("unexpected rollout require_commit_approval: %#v", rolloutCfg["require_commit_approval"])
	}
	if rolloutCfg["max_retry_attempts"] != float64(2) {
		t.Fatalf("unexpected rollout max_retry_attempts: %#v", rolloutCfg["max_retry_attempts"])
	}
	providers, _ := payload["providers"].(map[string]any)
	codex, _ := providers["codex"].(map[string]any)
	if codex["default_model"] != "gpt-5.3-codex" {
		t.Fatalf("unexpected codex default model: %#v", codex["default_model"])
	}
	if codex["approval_policy"] != "on-request" {
		t.Fatalf("unexpected codex approval policy: %#v", codex["approval_policy"])
	}
	keymap, _ := payload["keybindings"].(map[string]any)
	if keymap["ui.toggleSidebar"] != "alt+b" {
		t.Fatalf("unexpected keybinding override: %#v", keymap["ui.toggleSidebar"])
	}
	if got := payload["workflow_templates_path"]; got != filepath.Join(dataDir, "workflow_templates.json") {
		t.Fatalf("unexpected workflow_templates_path: %#v", got)
	}
	workflowCfg, _ := payload["workflow_templates"].(map[string]any)
	templates, _ := workflowCfg["templates"].([]any)
	if len(templates) != 1 {
		t.Fatalf("expected one workflow template, got %#v", workflowCfg["templates"])
	}
	template, _ := templates[0].(map[string]any)
	if template["id"] != "custom_delivery" {
		t.Fatalf("unexpected workflow template id: %#v", template["id"])
	}
}

func TestConfigCommandFailsOnInvalidUIConfig(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	dataDir := filepath.Join(home, ".archon")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "ui.toml"), []byte("[keybindings\npath='x'"), 0o600); err != nil {
		t.Fatalf("WriteFile ui.toml: %v", err)
	}

	cmd := NewConfigCommand(&bytes.Buffer{}, &bytes.Buffer{})
	if err := cmd.Run(nil); err == nil {
		t.Fatalf("expected config command to fail on invalid ui.toml")
	}
}

func TestConfigCommandFailsOnInvalidKeybindingsJSON(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	dataDir := filepath.Join(home, ".archon")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	ui := []byte("[keybindings]\npath = \"broken.json\"\n")
	if err := os.WriteFile(filepath.Join(dataDir, "ui.toml"), ui, 0o600); err != nil {
		t.Fatalf("WriteFile ui.toml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "broken.json"), []byte("{bad"), 0o600); err != nil {
		t.Fatalf("WriteFile keybindings: %v", err)
	}

	cmd := NewConfigCommand(&bytes.Buffer{}, &bytes.Buffer{})
	if err := cmd.Run(nil); err == nil {
		t.Fatalf("expected config command to fail on invalid keybindings json")
	}
}

func TestConfigCommandRejectsUnknownFlag(t *testing.T) {
	cmd := NewConfigCommand(&bytes.Buffer{}, &bytes.Buffer{})
	if err := cmd.Run([]string{"--unknown"}); err == nil {
		t.Fatalf("expected unknown flag to fail")
	}
}

func TestConfigCommandRejectsInvalidFormat(t *testing.T) {
	cmd := NewConfigCommand(&bytes.Buffer{}, &bytes.Buffer{})
	if err := cmd.Run([]string{"--format", "yaml"}); err == nil {
		t.Fatalf("expected invalid format to fail")
	}
}

func TestConfigCommandPrintsTOML(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	dataDir := filepath.Join(home, ".archon")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	content := []byte("[daemon]\naddress = \"127.0.0.1:7777\"\n")
	if err := os.WriteFile(filepath.Join(dataDir, "config.toml"), content, 0o600); err != nil {
		t.Fatalf("WriteFile config.toml: %v", err)
	}

	stdout := &bytes.Buffer{}
	cmd := NewConfigCommand(stdout, &bytes.Buffer{})
	if err := cmd.Run([]string{"--format", "toml"}); err != nil {
		t.Fatalf("config command failed: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "[daemon]") || !strings.Contains(out, "address =") || !strings.Contains(out, "127.0.0.1:7777") {
		t.Fatalf("expected daemon config in toml output, got %q", out)
	}
}

func TestConfigCommandDefaultIgnoresInvalidUserFiles(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	dataDir := filepath.Join(home, ".archon")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "config.toml"), []byte("[daemon\naddress='bad'"), 0o600); err != nil {
		t.Fatalf("WriteFile config.toml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "ui.toml"), []byte("[keybindings\npath='bad'"), 0o600); err != nil {
		t.Fatalf("WriteFile ui.toml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "workflow_templates.json"), []byte("{bad"), 0o600); err != nil {
		t.Fatalf("WriteFile workflow_templates.json: %v", err)
	}

	stdout := &bytes.Buffer{}
	cmd := NewConfigCommand(stdout, &bytes.Buffer{})
	if err := cmd.Run([]string{"--default"}); err != nil {
		t.Fatalf("expected --default to bypass malformed files, got %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json: %v raw=%q", err, stdout.String())
	}
	daemon, _ := payload["daemon"].(map[string]any)
	if daemon["address"] != "127.0.0.1:7777" {
		t.Fatalf("expected default daemon address, got %#v", daemon["address"])
	}
}

func TestConfigCommandScopeCoreSkipsInvalidUI(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	dataDir := filepath.Join(home, ".archon")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "ui.toml"), []byte("[keybindings\npath='bad'"), 0o600); err != nil {
		t.Fatalf("WriteFile ui.toml: %v", err)
	}

	stdout := &bytes.Buffer{}
	cmd := NewConfigCommand(stdout, &bytes.Buffer{})
	if err := cmd.Run([]string{"--scope", "core"}); err != nil {
		t.Fatalf("expected core scope to ignore ui parse errors, got %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json: %v raw=%q", err, stdout.String())
	}
	if _, ok := payload["daemon"]; !ok {
		t.Fatalf("expected core daemon output")
	}
	if daemon, ok := payload["daemon"].(map[string]any); !ok || daemon["base_url"] != nil {
		t.Fatalf("expected core-scope daemon output without base_url, got %#v", payload["daemon"])
	}
}

func TestConfigCommandScopeUIOnly(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	dataDir := filepath.Join(home, ".archon")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "ui.toml"), []byte("[keybindings]\npath=\"keys.json\""), 0o600); err != nil {
		t.Fatalf("WriteFile ui.toml: %v", err)
	}

	stdout := &bytes.Buffer{}
	cmd := NewConfigCommand(stdout, &bytes.Buffer{})
	if err := cmd.Run([]string{"--scope", "ui"}); err != nil {
		t.Fatalf("config command failed: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json: %v raw=%q", err, stdout.String())
	}
	keybindings, ok := payload["keybindings"].(map[string]any)
	if !ok {
		t.Fatalf("expected keybindings object in ui-only output, got %#v", payload["keybindings"])
	}
	if path, _ := keybindings["path"].(string); path == "" {
		t.Fatalf("expected keybindings.path in ui-only output")
	}
	chat, ok := payload["chat"].(map[string]any)
	if !ok {
		t.Fatalf("expected chat object in ui-only output, got %#v", payload["chat"])
	}
	if chat["timestamp_mode"] != "relative" {
		t.Fatalf("expected default chat timestamp mode relative, got %#v", chat["timestamp_mode"])
	}
}

func TestConfigCommandScopeKeybindingsDefault(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)

	stdout := &bytes.Buffer{}
	cmd := NewConfigCommand(stdout, &bytes.Buffer{})
	if err := cmd.Run([]string{"--scope", "keybindings", "--default"}); err != nil {
		t.Fatalf("config command failed: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json: %v raw=%q", err, stdout.String())
	}
	if payload["ui.toggleSidebar"] != "ctrl+b" {
		t.Fatalf("expected top-level keybinding map, got %#v", payload["ui.toggleSidebar"])
	}
	if payload["ui.startGuidedWorkflow"] != "w" {
		t.Fatalf("expected start guided workflow default key w, got %#v", payload["ui.startGuidedWorkflow"])
	}
	if _, ok := payload["keybindings"]; ok {
		t.Fatalf("did not expect nested keybindings object in keybindings-only output")
	}
	if _, ok := payload["keybindings_path"]; ok {
		t.Fatalf("did not expect keybindings_path metadata in keybindings-only output")
	}
}

func TestConfigCommandScopeWorkflowTemplatesOnly(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	dataDir := filepath.Join(home, ".archon")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	workflowTemplates := []byte(`{
  "version": 1,
  "templates": [
    {
      "id": "custom_delivery",
      "name": "Custom Delivery",
      "phases": [
        {
          "id": "phase_1",
          "name": "Plan",
          "steps": [
            {
              "id": "step_1",
              "name": "Draft plan",
              "prompt": "Draft a plan."
            }
          ]
        }
      ]
    }
  ]
}`)
	if err := os.WriteFile(filepath.Join(dataDir, "workflow_templates.json"), workflowTemplates, 0o600); err != nil {
		t.Fatalf("WriteFile workflow_templates.json: %v", err)
	}

	stdout := &bytes.Buffer{}
	cmd := NewConfigCommand(stdout, &bytes.Buffer{})
	if err := cmd.Run([]string{"--scope", "workflow_templates"}); err != nil {
		t.Fatalf("config command failed: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json: %v raw=%q", err, stdout.String())
	}
	templates, _ := payload["templates"].([]any)
	if len(templates) != 1 {
		t.Fatalf("expected top-level templates array, got %#v", payload["templates"])
	}
	if _, ok := payload["workflow_templates"]; ok {
		t.Fatalf("did not expect nested workflow_templates object in workflow_templates-only output")
	}
	if _, ok := payload["workflow_templates_path"]; ok {
		t.Fatalf("did not expect workflow_templates_path metadata in workflow_templates-only output")
	}
}

func TestConfigCommandScopeWorkflowTemplatesDefaultUsesRepoDefaults(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)

	stdout := &bytes.Buffer{}
	cmd := NewConfigCommand(stdout, &bytes.Buffer{})
	if err := cmd.Run([]string{"--scope", "workflow_templates", "--default"}); err != nil {
		t.Fatalf("config command failed: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json: %v raw=%q", err, stdout.String())
	}
	templates, _ := payload["templates"].([]any)
	if len(templates) == 0 {
		t.Fatalf("expected repo defaults in workflow_templates --default output")
	}
	first, _ := templates[0].(map[string]any)
	if first["id"] != "solid_phase_delivery" {
		t.Fatalf("unexpected default workflow template id: %#v", first["id"])
	}
}

func TestConfigCommandRejectsInvalidScope(t *testing.T) {
	cmd := NewConfigCommand(&bytes.Buffer{}, &bytes.Buffer{})
	if err := cmd.Run([]string{"--scope", "notes"}); err == nil {
		t.Fatalf("expected invalid scope to fail")
	}
}

func TestConfigCommandOmitsUnsetNetworkAccess(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	dataDir := filepath.Join(home, ".archon")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	content := []byte(`
[providers.codex]
default_model = "gpt-5.2-codex"
`)
	if err := os.WriteFile(filepath.Join(dataDir, "config.toml"), content, 0o600); err != nil {
		t.Fatalf("WriteFile config.toml: %v", err)
	}

	stdout := &bytes.Buffer{}
	cmd := NewConfigCommand(stdout, &bytes.Buffer{})
	if err := cmd.Run(nil); err != nil {
		t.Fatalf("config command failed: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json: %v raw=%q", err, stdout.String())
	}
	providersRaw, ok := payload["providers"].(map[string]any)
	if !ok {
		t.Fatalf("providers payload missing or invalid")
	}
	codexRaw, ok := providersRaw["codex"].(map[string]any)
	if !ok {
		t.Fatalf("codex payload missing or invalid")
	}
	if _, exists := codexRaw["network_access"]; exists {
		t.Fatalf("expected network_access to be omitted when unset")
	}
}

func TestConfigCommandPropagatesEncodeError(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".archon"), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	cmd := NewConfigCommand(errorWriter{}, &bytes.Buffer{})
	if err := cmd.Run(nil); err == nil {
		t.Fatalf("expected encoding error")
	}
}

func TestBuildCommandsIncludesConfig(t *testing.T) {
	wiring := commandWiring{
		stdout:             &bytes.Buffer{},
		stderr:             &bytes.Buffer{},
		newCloudAuthClient: fixedCloudAuthFactory(&fakeCommandClient{}),
		newSessionClient:   fixedSessionFactory(&fakeCommandClient{}),
		newUIClient:        fixedDaemonVersionFactory(&fakeCommandClient{}),
		newDaemonAdmin:     fixedDaemonAdminFactory(&fakeCommandClient{}),
		runDaemon:          func(bool) error { return nil },
		killDaemon:         func() error { return nil },
		configureUILogging: func() {},
		version:            "v-test",
	}
	commands := buildCommands(wiring)
	required := []string{"daemon", "config", "login", "whoami", "logout", "ps", "start", "kill", "tail", "ui"}
	for _, name := range required {
		if commands[name] == nil {
			t.Fatalf("expected %q command to be present", name)
		}
	}
	if _, ok := commands["config"].(*ConfigCommand); !ok {
		t.Fatalf("expected config command type")
	}
}

func TestDefaultCommandWiringUsesStandardStreams(t *testing.T) {
	wiring := defaultCommandWiring(nil, nil)
	if wiring.stdout != os.Stdout {
		t.Fatalf("expected stdout fallback to os.Stdout")
	}
	if wiring.stderr != os.Stderr {
		t.Fatalf("expected stderr fallback to os.Stderr")
	}
	if wiring.newCloudAuthClient == nil || wiring.newSessionClient == nil || wiring.newUIClient == nil || wiring.newDaemonAdmin == nil || wiring.runDaemon == nil || wiring.killDaemon == nil || wiring.configureUILogging == nil {
		t.Fatalf("expected default wiring to populate dependencies")
	}
}

type errorWriter struct{}

func (errorWriter) Write([]byte) (int, error) {
	return 0, fmt.Errorf("write failure")
}

type fakeCommandClient struct {
	ensureDaemonErr error

	ensureDaemonCalls     int
	ensureVersionErr      error
	ensureVersionCalls    int
	ensureVersionExpected string
	ensureVersionRestart  bool

	cloudLoginStartErr   error
	cloudLoginStartResp  *controlclient.CloudDeviceAuthorizationResponse
	cloudLoginStartCalls int

	cloudPollErrs      []error
	cloudPollResponses []*controlclient.CloudAuthPollResponse
	cloudPollCalls     int

	cloudStatusErr  error
	cloudStatusResp *controlclient.CloudAuthStatusResponse

	logoutCloudErr   error
	logoutCloudResp  *controlclient.CloudLogoutResponse
	logoutCloudCalls int

	listSessionsErr   error
	listSessionsCalls int
	sessionsResp      []*types.Session

	startSessionErr  error
	startSessionResp *types.Session
	startRequests    []controlclient.StartSessionRequest

	killSessionErr   error
	killSessionCalls int

	tailItemsErr   error
	tailItemsResp  *controlclient.TailItemsResponse
	tailItemsCalls int
	tailItemsID    string
	tailItemsLines int

	shutdownErr error
	healthErr   error
	healthResp  *controlclient.HealthResponse
	runUIErr    error
	runUICalls  int
}

func (f *fakeCommandClient) EnsureDaemon(context.Context) error {
	f.ensureDaemonCalls++
	return f.ensureDaemonErr
}

func (f *fakeCommandClient) EnsureDaemonVersion(_ context.Context, expectedVersion string, restart bool) error {
	f.ensureVersionCalls++
	f.ensureVersionExpected = expectedVersion
	f.ensureVersionRestart = restart
	return f.ensureVersionErr
}

func (f *fakeCommandClient) StartCloudLogin(context.Context) (*controlclient.CloudDeviceAuthorizationResponse, error) {
	f.cloudLoginStartCalls++
	if f.cloudLoginStartErr != nil {
		return nil, f.cloudLoginStartErr
	}
	if f.cloudLoginStartResp == nil {
		return nil, errors.New("cloudLoginStartResp not configured")
	}
	return f.cloudLoginStartResp, nil
}

func (f *fakeCommandClient) PollCloudLogin(context.Context) (*controlclient.CloudAuthPollResponse, error) {
	index := f.cloudPollCalls
	f.cloudPollCalls++
	if index < len(f.cloudPollErrs) && f.cloudPollErrs[index] != nil {
		return nil, f.cloudPollErrs[index]
	}
	if index < len(f.cloudPollResponses) && f.cloudPollResponses[index] != nil {
		return f.cloudPollResponses[index], nil
	}
	return nil, errors.New("cloudPollResponses not configured")
}

func (f *fakeCommandClient) CloudAuthStatus(context.Context) (*controlclient.CloudAuthStatusResponse, error) {
	if f.cloudStatusErr != nil {
		return nil, f.cloudStatusErr
	}
	if f.cloudStatusResp == nil {
		return &controlclient.CloudAuthStatusResponse{}, nil
	}
	return f.cloudStatusResp, nil
}

func (f *fakeCommandClient) LogoutCloud(context.Context) (*controlclient.CloudLogoutResponse, error) {
	f.logoutCloudCalls++
	if f.logoutCloudErr != nil {
		return nil, f.logoutCloudErr
	}
	if f.logoutCloudResp == nil {
		return &controlclient.CloudLogoutResponse{Status: "revoked_and_unlinked", Message: "revoked remote token and cleared local cloud credentials", RemoteRevoked: true, LocalCleared: true}, nil
	}
	return f.logoutCloudResp, nil
}

func (f *fakeCommandClient) ListSessions(context.Context) ([]*types.Session, error) {
	f.listSessionsCalls++
	return f.sessionsResp, f.listSessionsErr
}

func (f *fakeCommandClient) StartSession(_ context.Context, req controlclient.StartSessionRequest) (*types.Session, error) {
	f.startRequests = append(f.startRequests, req)
	if f.startSessionErr != nil {
		return nil, f.startSessionErr
	}
	if f.startSessionResp == nil {
		return nil, errors.New("startSessionResp not configured")
	}
	return f.startSessionResp, nil
}

func (f *fakeCommandClient) KillSession(context.Context, string) error {
	f.killSessionCalls++
	return f.killSessionErr
}

func (f *fakeCommandClient) TailItems(_ context.Context, id string, lines int) (*controlclient.TailItemsResponse, error) {
	f.tailItemsCalls++
	f.tailItemsID = id
	f.tailItemsLines = lines
	if f.tailItemsErr != nil {
		return nil, f.tailItemsErr
	}
	if f.tailItemsResp == nil {
		return nil, errors.New("tailItemsResp not configured")
	}
	return f.tailItemsResp, nil
}

func (f *fakeCommandClient) ShutdownDaemon(context.Context) error {
	return f.shutdownErr
}

func (f *fakeCommandClient) Health(context.Context) (*controlclient.HealthResponse, error) {
	return f.healthResp, f.healthErr
}

func (f *fakeCommandClient) RunUI() error {
	f.runUICalls++
	return f.runUIErr
}

func fixedCloudAuthFactory(client cloudAuthCommandClient) cloudAuthClientFactory {
	return func() (cloudAuthCommandClient, error) {
		return client, nil
	}
}

func fixedSessionFactory(client sessionCommandClient) sessionClientFactory {
	return func() (sessionCommandClient, error) {
		return client, nil
	}
}

func fixedDaemonVersionFactory(client daemonVersionClient) daemonVersionClientFactory {
	return func() (daemonVersionClient, error) {
		return client, nil
	}
}

func fixedDaemonAdminFactory(client daemonAdminClient) daemonAdminClientFactory {
	return func() (daemonAdminClient, error) {
		return client, nil
	}
}
