package daemon

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"control/internal/config"
	"control/internal/daemon/transcriptdomain"
	"control/internal/guidedworkflows"
	"control/internal/logging"
	"control/internal/providers"
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

var workflowContextCarryTemplate = guidedworkflows.WorkflowTemplate{
	ID:   "gwf_e2e_context_carry",
	Name: "E2E Context Carry Arithmetic",
	Phases: []guidedworkflows.WorkflowTemplatePhase{
		{
			ID:   "phase_chain",
			Name: "Chain",
			Steps: []guidedworkflows.WorkflowTemplateStep{
				{
					ID:     "step_multiply",
					Name:   "Multiply",
					Prompt: "Take the number from the user message and multiply it by 2. Respond only with the result.",
				},
				{
					ID:     "step_add",
					Name:   "Add",
					Prompt: "Take the number you gave me and add 5 to it. Respond only with the result.",
				},
			},
		},
	},
}

const workflowIntegrationPollInterval = 100 * time.Millisecond

func TestGuidedWorkflowE2E(t *testing.T) {
	// Dispatch-capable provider coverage is centralized by
	// guidedWorkflowDispatchProviderProfiles(), which reuses allProviderTestCases
	// for shared require/setup/timeout behavior and env gating.
	profiles := guidedWorkflowDispatchProviderProfiles()
	requireGuidedWorkflowProviderCoverage(t, profiles, "TestGuidedWorkflowE2E")
	for _, profile := range profiles {
		t.Run(profile.name(), func(t *testing.T) {
			t.Parallel()
			profile.require(t)
			repoDir, runtimeOpts := profile.setup(t)
			runWorkflowE2E(t, profile, repoDir, runtimeOpts, guidedWorkflowTimeout(profile))
		})
	}
}

func TestGuidedWorkflowE2EContextCarryArithmetic(t *testing.T) {
	// This scenario intentionally mirrors TestGuidedWorkflowE2E provider coverage
	// so context carry validation stays at parity for all dispatch-capable providers.
	profiles := guidedWorkflowDispatchProviderProfiles()
	requireGuidedWorkflowProviderCoverage(t, profiles, "TestGuidedWorkflowE2EContextCarryArithmetic")
	base := workflowContextCarryBaseNumber()
	for _, profile := range profiles {
		t.Run(profile.name(), func(t *testing.T) {
			t.Parallel()
			profile.require(t)
			repoDir, runtimeOpts := profile.setup(t)
			runWorkflowContextCarryE2E(t, profile, repoDir, runtimeOpts, base, guidedWorkflowTimeout(profile))
		})
	}
}

func TestGuidedWorkflowE2EInvalidModelFails(t *testing.T) {
	// Invalid-model failures are asserted only for providers where model catalogs
	// and API error semantics make this scenario meaningful and stable.
	profiles := guidedWorkflowInvalidModelProviderProfiles()
	requireGuidedWorkflowProviderCoverage(t, profiles, "TestGuidedWorkflowE2EInvalidModelFails")
	for _, profile := range profiles {
		t.Run(profile.name(), func(t *testing.T) {
			t.Parallel()
			profile.require(t)

			repoDir, _ := profile.setup(t)
			server, _, _ := newWorkflowIntegrationServer(t)
			defer server.Close()

			ws := createWorkspace(t, server, repoDir)
			created := createWorkflowRunViaAPI(t, server, CreateWorkflowRunRequest{
				TemplateID:       workflowE2ETemplate.ID,
				WorkspaceID:      ws.ID,
				UserPrompt:       "Follow the instructions exactly.",
				SelectedProvider: profile.name(),
				SelectedRuntimeOptions: &types.SessionRuntimeOptions{
					Model: "archon/not-a-real-model",
				},
			})
			postWorkflowRunAction(t, server, created.ID, "start", http.StatusInternalServerError)

			timeout := guidedWorkflowInvalidModelTimeout(profile)
			waitForWorkflowRunStatus(t, server, created.ID, guidedworkflows.WorkflowRunStatusFailed, timeout)

			run := getWorkflowRun(t, server, created.ID, http.StatusOK)
			if !strings.Contains(strings.ToLower(run.LastError), "not supported") &&
				!strings.Contains(strings.ToLower(run.LastError), "invalid model") {
				t.Fatalf("expected invalid model failure detail, got %q", run.LastError)
			}
		})
	}
}

func runWorkflowE2E(t *testing.T, profile guidedWorkflowProviderProfile, repoDir string, runtimeOpts *types.SessionRuntimeOptions, timeout time.Duration) {
	t.Helper()

	server, manager, _ := newWorkflowIntegrationServer(t)
	defer server.Close()

	ws := createWorkspace(t, server, repoDir)

	// Create and start the workflow run.
	created := createWorkflowRunViaAPI(t, server, CreateWorkflowRunRequest{
		TemplateID:             workflowE2ETemplate.ID,
		WorkspaceID:            ws.ID,
		UserPrompt:             "Follow the instructions exactly.",
		SelectedProvider:       profile.name(),
		SelectedRuntimeOptions: runtimeOpts,
	})
	startWorkflowRunOrSkipIfUnavailable(t, server, created.ID, profile.name())

	// Wait for the workflow to bind a session.
	sessionID := waitForWorkflowSession(t, server, created.ID, timeout)

	// Phase 1: wait for the agent to reply with a number.
	number := waitForNumberReplyInRange(t, server, manager, profile.name(), profile.replyTransport, sessionID, 1, 100, timeout)
	t.Logf("phase 1: agent picked %d", number)

	// Phase 2: wait for the agent to reply with 2*number.
	expected := number * 2
	waitForSpecificNumberReply(t, server, manager, profile.name(), profile.replyTransport, sessionID, expected, timeout)
	t.Logf("phase 2: agent replied with %d", expected)

	// Verify workflow reached completed status.
	waitForWorkflowRunStatus(t, server, created.ID, guidedworkflows.WorkflowRunStatusCompleted, timeout)
}

func runWorkflowContextCarryE2E(t *testing.T, profile guidedWorkflowProviderProfile, repoDir string, runtimeOpts *types.SessionRuntimeOptions, base int, timeout time.Duration) {
	t.Helper()

	server, manager, _ := newWorkflowIntegrationServer(t)
	defer server.Close()

	ws := createWorkspace(t, server, repoDir)

	created := createWorkflowRunViaAPI(t, server, CreateWorkflowRunRequest{
		TemplateID:             workflowContextCarryTemplate.ID,
		WorkspaceID:            ws.ID,
		UserPrompt:             "Use this number exactly: " + strconv.Itoa(base),
		SelectedProvider:       profile.name(),
		SelectedRuntimeOptions: runtimeOpts,
	})
	startWorkflowRunOrSkipIfUnavailable(t, server, created.ID, profile.name())

	sessionID := waitForWorkflowSession(t, server, created.ID, timeout)

	step1 := base * 2
	waitForSpecificNumberReply(t, server, manager, profile.name(), profile.replyTransport, sessionID, step1, timeout)
	t.Logf("step 1: agent replied with %d", step1)

	final := step1 + 5
	waitForSpecificNumberReply(t, server, manager, profile.name(), profile.replyTransport, sessionID, final, timeout)
	t.Logf("step 2: agent replied with %d", final)

	waitForWorkflowRunStatus(t, server, created.ID, guidedworkflows.WorkflowRunStatusCompleted, timeout)
}

// newWorkflowIntegrationServer builds an httptest.Server with the full
// turn-completion → workflow-engine feedback loop wired up, mirroring the
// production sequence in daemon.go:131-201.
func newWorkflowIntegrationServer(t *testing.T) (*httptest.Server, *SessionManager, *Stores) {
	t.Helper()

	base := newDaemonIntegrationTempDir(t, "workflow-server-*")
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
	artifactRepository := newFileSessionItemsRepository(manager)
	compositeLive := NewCompositeLiveManager(stores, logger,
		newCodexLiveSessionFactory(liveCodex),
		newClaudeLiveSessionFactory(manager, stores, nil, turnNotifier, logger),
		newOpenCodeLiveSessionFactory("opencode", turnNotifier, approvalStore, artifactRepository, nil, nil, logger),
		newOpenCodeLiveSessionFactory("kilocode", turnNotifier, approvalStore, artifactRepository, nil, nil, logger),
	)

	// Build workflow RunService with a real prompt dispatcher and the test template.
	dispatcher := newGuidedWorkflowPromptDispatcher(coreCfg, manager, stores, compositeLive, logger)
	workflowRuns := guidedworkflows.NewRunService(
		guidedworkflows.Config{Enabled: true},
		guidedworkflows.WithTemplate(workflowE2ETemplate),
		guidedworkflows.WithTemplate(workflowContextCarryTemplate),
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
		time.Sleep(workflowIntegrationPollInterval)
	}
	t.Fatalf("timeout waiting for workflow run %s to bind a session", runID)
	return ""
}

func startWorkflowRunOrSkipIfUnavailable(t *testing.T, server *httptest.Server, runID string, provider string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/workflow-runs/"+runID+"/start", bytes.NewReader(nil))
	req.Header.Set("Authorization", "Bearer token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("workflow action request: %v", err)
	}
	defer closeTestCloser(t, resp.Body)

	payload, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusOK {
		return
	}

	body := strings.TrimSpace(string(payload))
	if isWorkflowDispatchUnavailableIntegrationSkip(provider, resp.StatusCode, body) {
		t.Skipf("skipping guided workflow provider %q: start unavailable in this environment (%s)", provider, body)
	}
	t.Fatalf("unexpected status for start: got=%d want=%d payload=%s", resp.StatusCode, http.StatusOK, body)
}

func isWorkflowDispatchUnavailableIntegrationSkip(provider string, status int, payload string) bool {
	if status != http.StatusInternalServerError {
		return false
	}
	switch providers.Normalize(provider) {
	case "opencode", "kilocode":
		return strings.Contains(strings.ToLower(payload), "workflow step prompt dispatch unavailable")
	default:
		return false
	}
}

var strictNumberPattern = regexp.MustCompile(`^\s*(\d{1,6})\s*$`)

func waitForNumberReplyInRange(
	t *testing.T,
	server *httptest.Server,
	manager *SessionManager,
	provider string,
	replyTransport guidedWorkflowReplyTransport,
	sessionID string,
	min int,
	max int,
	timeout time.Duration,
) int {
	t.Helper()
	requireGuidedWorkflowReplyTransport(t, provider, replyTransport)
	if replyTransport == guidedWorkflowReplyTransportItems {
		return waitForNumberInItemsRange(t, server, manager, provider, sessionID, min, max, timeout)
	}
	return waitForNumberInHistoryRange(t, server, manager, provider, sessionID, min, max, timeout)
}

func waitForSpecificNumberReply(
	t *testing.T,
	server *httptest.Server,
	manager *SessionManager,
	provider string,
	replyTransport guidedWorkflowReplyTransport,
	sessionID string,
	expected int,
	timeout time.Duration,
) {
	t.Helper()
	requireGuidedWorkflowReplyTransport(t, provider, replyTransport)
	if replyTransport == guidedWorkflowReplyTransportItems {
		waitForSpecificNumberReplyFromItems(t, server, manager, provider, sessionID, expected, timeout)
		return
	}
	waitForSpecificNumberReplyFromHistory(t, server, manager, provider, sessionID, expected, timeout)
}

func waitForNumberInHistoryRange(t *testing.T, server *httptest.Server, manager *SessionManager, provider, sessionID string, min, max int, timeout time.Duration) int {
	t.Helper()
	failures, stopFailures := startSessionTurnFailureMonitor(server, sessionID)
	defer stopFailures()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if failure := sessionTerminalFailure(server, sessionID); failure != "" {
			maybeSkipGuidedWorkflowProviderFailure(t, provider, failure)
			t.Fatalf("session entered terminal failure state before numeric reply: %s\n%s", failure, sessionDiagnostics(manager, sessionID))
		}
		select {
		case failure, ok := <-failures:
			if ok && strings.TrimSpace(failure) != "" {
				maybeSkipGuidedWorkflowProviderFailure(t, provider, failure)
				t.Fatalf("provider turn failed before numeric reply: %s\n%s", failure, sessionDiagnostics(manager, sessionID))
			}
		default:
		}
		history := historySession(t, server, sessionID)
		if n, ok := findStrictNumberInHistoryItems(history.Items, min, max); ok {
			return n
		}
		time.Sleep(workflowIntegrationPollInterval)
	}
	diag := sessionDiagnostics(manager, sessionID)
	maybeSkipGuidedWorkflowProviderFailure(t, provider, diag)
	t.Fatalf("timeout waiting for agent numeric reply in [%d,%d]\n%s", min, max, diag)
	return 0
}

func waitForNumberInItemsRange(t *testing.T, server *httptest.Server, manager *SessionManager, provider, sessionID string, min, max int, timeout time.Duration) int {
	t.Helper()
	stream, closeFn := openSSE(t, server, "/v1/sessions/"+sessionID+"/transcript/stream?follow=1")
	defer closeFn()

	failures, stopFailures := startSessionTurnFailureMonitor(server, sessionID)
	defer stopFailures()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if failure := sessionTerminalFailure(server, sessionID); failure != "" {
			maybeSkipGuidedWorkflowProviderFailure(t, provider, failure)
			t.Fatalf("session entered terminal failure state before numeric reply: %s\n%s", failure, sessionDiagnostics(manager, sessionID))
		}
		data, failure, ok := waitForSSEDataWithFailure(stream, failures, 5*time.Second)
		if strings.TrimSpace(failure) != "" {
			maybeSkipGuidedWorkflowProviderFailure(t, provider, failure)
			t.Fatalf("provider turn failed before numeric reply: %s\n%s", failure, sessionDiagnostics(manager, sessionID))
		}
		if !ok {
			continue
		}
		var event transcriptdomain.TranscriptEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}
		n, ok := strictNumberFromTranscriptEvent(event)
		if ok && n >= min && n <= max {
			return n
		}
	}
	diag := sessionDiagnostics(manager, sessionID)
	maybeSkipGuidedWorkflowProviderFailure(t, provider, diag)
	t.Fatalf("timeout waiting for agent numeric reply in [%d,%d]\n%s", min, max, diag)
	return 0
}

func waitForSpecificNumberReplyFromHistory(t *testing.T, server *httptest.Server, manager *SessionManager, provider, sessionID string, expected int, timeout time.Duration) {
	t.Helper()
	failures, stopFailures := startSessionTurnFailureMonitor(server, sessionID)
	defer stopFailures()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if failure := sessionTerminalFailure(server, sessionID); failure != "" {
			maybeSkipGuidedWorkflowProviderFailure(t, provider, failure)
			t.Fatalf("session entered terminal failure state before numeric reply %d: %s\n%s", expected, failure, sessionDiagnostics(manager, sessionID))
		}
		select {
		case failure, ok := <-failures:
			if ok && strings.TrimSpace(failure) != "" {
				maybeSkipGuidedWorkflowProviderFailure(t, provider, failure)
				t.Fatalf("provider turn failed before numeric reply %d: %s\n%s", expected, failure, sessionDiagnostics(manager, sessionID))
			}
		default:
		}
		history := historySession(t, server, sessionID)
		if hasExpectedNumberInHistoryItems(history.Items, expected) {
			return
		}
		time.Sleep(workflowIntegrationPollInterval)
	}
	diag := sessionDiagnostics(manager, sessionID)
	maybeSkipGuidedWorkflowProviderFailure(t, provider, diag)
	t.Fatalf("timeout waiting for numeric reply %d\n%s", expected, diag)
}

func waitForSpecificNumberReplyFromItems(t *testing.T, server *httptest.Server, manager *SessionManager, provider, sessionID string, expected int, timeout time.Duration) {
	t.Helper()
	stream, closeFn := openSSE(t, server, "/v1/sessions/"+sessionID+"/transcript/stream?follow=1")
	defer closeFn()

	failures, stopFailures := startSessionTurnFailureMonitor(server, sessionID)
	defer stopFailures()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if failure := sessionTerminalFailure(server, sessionID); failure != "" {
			maybeSkipGuidedWorkflowProviderFailure(t, provider, failure)
			t.Fatalf("session entered terminal failure state before numeric reply %d: %s\n%s", expected, failure, sessionDiagnostics(manager, sessionID))
		}
		data, failure, ok := waitForSSEDataWithFailure(stream, failures, 5*time.Second)
		if strings.TrimSpace(failure) != "" {
			maybeSkipGuidedWorkflowProviderFailure(t, provider, failure)
			t.Fatalf("provider turn failed before numeric reply %d: %s\n%s", expected, failure, sessionDiagnostics(manager, sessionID))
		}
		if !ok {
			continue
		}
		var event transcriptdomain.TranscriptEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}
		if transcriptEventContainsExpectedNumber(event, expected) {
			return
		}
	}
	diag := sessionDiagnostics(manager, sessionID)
	maybeSkipGuidedWorkflowProviderFailure(t, provider, diag)
	t.Fatalf("timeout waiting for numeric reply %d\n%s", expected, diag)
}

func extractStrictNumber(text string) (int, bool) {
	match := strictNumberPattern.FindStringSubmatch(strings.TrimSpace(text))
	if len(match) != 2 {
		return 0, false
	}
	n, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, false
	}
	return n, true
}

func strictNumberFromHistoryItem(item map[string]any) (int, bool) {
	if !isAgentHistoryItem(item) {
		return 0, false
	}
	return extractStrictNumber(extractHistoryText(item))
}

func strictNumberFromTranscriptEvent(event transcriptdomain.TranscriptEvent) (int, bool) {
	if event.Kind != transcriptdomain.TranscriptEventDelta {
		return 0, false
	}
	for _, block := range event.Delta {
		role := strings.ToLower(strings.TrimSpace(block.Role))
		if role != "assistant" && role != "agent" && role != "model" {
			continue
		}
		if n, ok := extractStrictNumber(block.Text); ok {
			return n, true
		}
	}
	return 0, false
}

func transcriptEventContainsExpectedNumber(event transcriptdomain.TranscriptEvent, expected int) bool {
	n, ok := strictNumberFromTranscriptEvent(event)
	return ok && n == expected
}

func findStrictNumberInHistoryItems(items []map[string]any, min, max int) (int, bool) {
	for _, item := range items {
		n, ok := strictNumberFromHistoryItem(item)
		if ok && n >= min && n <= max {
			return n, true
		}
	}
	return 0, false
}

func hasStrictNumberInHistoryItems(items []map[string]any, expected int) bool {
	for _, item := range items {
		n, ok := strictNumberFromHistoryItem(item)
		if ok && n == expected {
			return true
		}
	}
	return false
}

func hasExpectedNumberInHistoryItems(items []map[string]any, expected int) bool {
	for _, item := range items {
		if historyItemContainsExpectedNumber(item, expected) {
			return true
		}
	}
	return false
}

func historyItemContainsExpectedNumber(item map[string]any, expected int) bool {
	if !isAgentHistoryItem(item) {
		return false
	}
	text := extractHistoryText(item)
	if text == "" {
		return false
	}
	pattern := `\b` + regexp.QuoteMeta(strconv.Itoa(expected)) + `\b`
	return regexp.MustCompile(pattern).MatchString(text)
}

func isAgentHistoryItem(item map[string]any) bool {
	if item == nil {
		return false
	}
	typ, _ := item["type"].(string)
	return typ == "agentMessage" || typ == "agentMessageDelta" || typ == "assistant"
}

func maybeSkipGuidedWorkflowProviderFailure(t *testing.T, provider string, failure string) {
	t.Helper()
	if !isGuidedWorkflowProviderRuntimeUnavailableFailure(provider, failure) {
		return
	}
	t.Skipf("skipping guided workflow provider %q: runtime/auth not ready (%s)", provider, strings.TrimSpace(failure))
}

func isGuidedWorkflowProviderRuntimeUnavailableFailure(provider string, failure string) bool {
	switch providers.Normalize(provider) {
	case "opencode", "kilocode":
	default:
		return false
	}
	failure = strings.ToLower(strings.TrimSpace(failure))
	if failure == "" {
		return false
	}
	return strings.Contains(failure, "user not found") ||
		strings.Contains(failure, "unauthorized") ||
		strings.Contains(failure, "forbidden") ||
		strings.Contains(failure, "authentication") ||
		strings.Contains(failure, "not authenticated") ||
		strings.Contains(failure, "dispatch unavailable")
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
		time.Sleep(workflowIntegrationPollInterval)
	}
	t.Fatalf("timeout waiting for workflow run %s to reach %s (last=%s)", runID, want, last)
}

func workflowContextCarryBaseNumber() int {
	if raw := strings.TrimSpace(os.Getenv("ARCHON_WORKFLOW_CONTEXT_CARRY_NUMBER")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			return n
		}
	}
	return 12
}
