package daemon

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
	"time"

	"control/internal/config"
	"control/internal/guidedworkflows"
	"control/internal/logging"
	"control/internal/store"
	"control/internal/types"
)

var workflowE2ETemplate = guidedworkflows.WorkflowTemplate{
	ID:   "gwf_e2e_number_test",
	Name: "E2E Number Test",
	Phases: []guidedworkflows.WorkflowTemplatePhase{
		{
			ID:   "phase_pick",
			Name: "Pick Number",
			Steps: []guidedworkflows.WorkflowTemplateStep{
				{
					ID:     "step_pick",
					Name:   "Pick",
					Prompt: "Pick a number between 1 and 100. Reply with ONLY the number, nothing else.",
				},
			},
		},
		{
			ID:   "phase_multiply",
			Name: "Multiply",
			Steps: []guidedworkflows.WorkflowTemplateStep{
				{
					ID:     "step_multiply",
					Name:   "Multiply",
					Prompt: "Multiply the number you picked by 2. Reply with ONLY the result, nothing else.",
				},
			},
		},
	},
}

func TestGuidedWorkflowE2E(t *testing.T) {
	tests := []struct {
		name    string
		require func(t *testing.T)
		setup   func(t *testing.T) (repoDir string, opts *types.SessionRuntimeOptions)
	}{
		{
			name:    "codex",
			require: requireCodexIntegration,
			setup:   setupCodexWorkflow,
		},
		{
			name:    "claude",
			require: requireClaudeIntegration,
			setup:   setupClaudeWorkflow,
		},
		{
			name:    "opencode",
			require: func(t *testing.T) { requireOpenCodeIntegration(t, "opencode") },
			setup:   setupOpenCodeWorkflow("opencode"),
		},
		{
			name:    "kilocode",
			require: func(t *testing.T) { requireOpenCodeIntegration(t, "kilocode") },
			setup:   setupOpenCodeWorkflow("kilocode"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.require(t)
			repoDir, runtimeOpts := tc.setup(t)
			runWorkflowE2E(t, tc.name, repoDir, runtimeOpts)
		})
	}
}

func runWorkflowE2E(t *testing.T, provider, repoDir string, runtimeOpts *types.SessionRuntimeOptions) {
	t.Helper()

	server, manager, _ := newWorkflowIntegrationServer(t)
	defer server.Close()

	ws := createWorkspace(t, server, repoDir)
	timeout := workflowIntegrationTimeout(provider)

	// Create and start the workflow run.
	created := createWorkflowRunViaAPI(t, server, CreateWorkflowRunRequest{
		TemplateID:             workflowE2ETemplate.ID,
		WorkspaceID:            ws.ID,
		UserPrompt:             "Follow the instructions exactly.",
		SelectedProvider:       provider,
		SelectedRuntimeOptions: runtimeOpts,
	})
	postWorkflowRunAction(t, server, created.ID, "start", http.StatusOK)

	// Wait for the workflow to bind a session.
	sessionID := waitForWorkflowSession(t, server, created.ID, timeout)

	// Phase 1: wait for the agent to reply with a number.
	number := waitForNumberInHistory(t, server, manager, sessionID, timeout)
	t.Logf("phase 1: agent picked %d", number)

	// Phase 2: wait for the agent to reply with 2*number.
	expected := number * 2
	waitForAgentReply(t, server, manager, sessionID, strconv.Itoa(expected), timeout)
	t.Logf("phase 2: agent replied with %d", expected)

	// Verify workflow reached completed status.
	waitForWorkflowRunStatus(t, server, created.ID, guidedworkflows.WorkflowRunStatusCompleted, timeout)
}

// newWorkflowIntegrationServer builds an httptest.Server with the full
// turn-completion → workflow-engine feedback loop wired up, mirroring the
// production sequence in daemon.go:131-201.
func newWorkflowIntegrationServer(t *testing.T) (*httptest.Server, *SessionManager, *Stores) {
	t.Helper()

	base := t.TempDir()
	manager, err := NewSessionManager(filepath.Join(base, "sessions"))
	if err != nil {
		t.Fatalf("NewSessionManager: %v", err)
	}

	workspaces := store.NewFileWorkspaceStore(filepath.Join(base, "workspaces.json"))
	state := store.NewFileAppStateStore(filepath.Join(base, "state.json"))
	meta := store.NewFileSessionMetaStore(filepath.Join(base, "sessions_meta.json"))
	sessions := store.NewFileSessionIndexStore(filepath.Join(base, "sessions.json"))
	approvals := store.NewFileApprovalStore(filepath.Join(base, "approvals.json"))

	stores := &Stores{
		Workspaces:  workspaces,
		Worktrees:   workspaces,
		Groups:      workspaces,
		AppState:    state,
		SessionMeta: meta,
		Sessions:    sessions,
		Approvals:   approvals,
	}
	manager.SetMetaStore(meta)
	manager.SetSessionStore(sessions)

	logger := logging.New(io.Discard, logging.Debug)
	coreCfg := config.DefaultCoreConfig()

	// Build live session factories with turn notification support.
	liveCodex := NewCodexLiveManager(stores, logger)
	turnNotifier := NewTurnCompletionNotifier(nil, stores)
	approvalStore := NewStoreApprovalStorage(stores)
	compositeLive := NewCompositeLiveManager(stores, logger,
		newCodexLiveSessionFactory(liveCodex),
		newClaudeLiveSessionFactory(manager, stores, nil, turnNotifier, logger),
		newOpenCodeLiveSessionFactory("opencode", turnNotifier, approvalStore, nil, nil, nil, logger),
		newOpenCodeLiveSessionFactory("kilocode", turnNotifier, approvalStore, nil, nil, nil, logger),
	)

	// Build workflow RunService with a real prompt dispatcher and the test template.
	dispatcher := newGuidedWorkflowPromptDispatcher(coreCfg, manager, stores, compositeLive, logger)
	workflowRuns := guidedworkflows.NewRunService(
		guidedworkflows.Config{Enabled: true},
		guidedworkflows.WithTemplate(workflowE2ETemplate),
		guidedworkflows.WithStepPromptDispatcher(dispatcher),
	)

	// Close the turn-completion → workflow-engine feedback loop.
	var turnProcessor guidedworkflows.TurnEventProcessor
	if processor, ok := any(workflowRuns).(guidedworkflows.TurnEventProcessor); ok {
		turnProcessor = processor
	}
	eventPublisher := NewGuidedWorkflowNotificationPublisher(nil, nil, turnProcessor)
	compositeLive.SetNotificationPublisher(eventPublisher)
	liveCodex.SetNotificationPublisher(eventPublisher)
	turnNotifier.SetNotificationPublisher(eventPublisher)
	manager.SetNotificationPublisher(eventPublisher)

	api := &API{
		Version:      "test",
		Manager:      manager,
		Stores:       stores,
		Logger:       logger,
		LiveCodex:    liveCodex,
		LiveManager:  compositeLive,
		WorkflowRuns: workflowRuns,
	}

	mux := http.NewServeMux()
	api.RegisterRoutes(mux)
	server := httptest.NewServer(TokenAuthMiddleware("token", mux))
	return server, manager, stores
}

// ---------------------------------------------------------------------------
// Per-provider setup
// ---------------------------------------------------------------------------

func setupCodexWorkflow(t *testing.T) (string, *types.SessionRuntimeOptions) {
	t.Helper()
	repoDir, codexHome := createCodexWorkspace(t)
	requireCodexAuth(t, repoDir, codexHome)
	model := resolveCodexIntegrationModelForWorkspace(t, repoDir, codexHome)
	return repoDir, &types.SessionRuntimeOptions{Model: model}
}

func setupClaudeWorkflow(t *testing.T) (string, *types.SessionRuntimeOptions) {
	t.Helper()
	// Clear env vars that prevent the claude CLI from running inside a
	// Claude Code session. The nested invocation is a separate process
	// managed by the daemon, not a true recursive session.
	t.Setenv("CLAUDECODE", "")
	t.Setenv("CLAUDE_CODE_ENTRYPOINT", "")
	repoDir := filepath.Join(t.TempDir(), "claude-repo")
	if err := os.MkdirAll(repoDir, 0o700); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	return repoDir, nil
}

func setupOpenCodeWorkflow(provider string) func(t *testing.T) (string, *types.SessionRuntimeOptions) {
	return func(t *testing.T) (string, *types.SessionRuntimeOptions) {
		t.Helper()
		return createOpenCodeWorkspace(t, provider), nil
	}
}

// ---------------------------------------------------------------------------
// Workflow-specific helpers
// ---------------------------------------------------------------------------

func waitForWorkflowSession(t *testing.T, server *httptest.Server, runID string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		run := getWorkflowRun(t, server, runID, http.StatusOK)
		if run.SessionID != "" {
			return run.SessionID
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for workflow run %s to bind a session", runID)
	return ""
}

var numberPattern = regexp.MustCompile(`\b(\d{1,3})\b`)

func waitForNumberInHistory(t *testing.T, server *httptest.Server, manager *SessionManager, sessionID string, timeout time.Duration) int {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		history := historySession(t, server, sessionID)
		for _, item := range history.Items {
			typ, _ := item["type"].(string)
			if typ != "agentMessage" && typ != "agentMessageDelta" && typ != "assistant" {
				continue
			}
			text := extractHistoryText(item)
			if match := numberPattern.FindString(text); match != "" {
				n, err := strconv.Atoi(match)
				if err == nil && n >= 1 && n <= 100 {
					return n
				}
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for agent to pick a number\n%s", sessionDiagnostics(manager, sessionID))
	return 0
}

func waitForWorkflowRunStatus(t *testing.T, server *httptest.Server, runID string, want guidedworkflows.WorkflowRunStatus, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last guidedworkflows.WorkflowRunStatus
	for time.Now().Before(deadline) {
		run := getWorkflowRun(t, server, runID, http.StatusOK)
		last = run.Status
		if run.Status == want {
			return
		}
		if run.Status == guidedworkflows.WorkflowRunStatusFailed {
			t.Fatalf("workflow run %s failed (wanted %s)", runID, want)
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for workflow run %s to reach %s (last=%s)", runID, want, last)
}

func workflowIntegrationTimeout(provider string) time.Duration {
	switch provider {
	case "codex":
		return codexIntegrationTimeout() + 1*time.Minute
	case "opencode", "kilocode":
		return openCodeIntegrationTimeout(provider) + 1*time.Minute
	default:
		if raw := os.Getenv("ARCHON_WORKFLOW_TIMEOUT"); raw != "" {
			if d, err := time.ParseDuration(raw); err == nil {
				return d
			}
		}
		return 3 * time.Minute
	}
}
