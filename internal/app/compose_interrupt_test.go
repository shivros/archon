package app

import (
	"errors"
	"strings"
	"testing"
	"time"

	"control/internal/types"

	tea "charm.land/bubbletea/v2"
)

func newComposeInterruptTestModel(provider string) *Model {
	m := NewModel(nil)
	now := time.Now().UTC()
	m.sessions = []*types.Session{
		{
			ID:        "s1",
			Provider:  provider,
			Status:    types.SessionStatusRunning,
			CreatedAt: now,
		},
	}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", LastTurnID: "turn-1"},
	}
	m.resize(120, 40)
	m.enterCompose("s1")
	return &m
}

func findComposeSpan(spans []composeControlSpan, action composeControlAction) (composeControlSpan, bool) {
	for _, span := range spans {
		if span.action == action {
			return span, true
		}
	}
	return composeControlSpan{}, false
}

func TestComposeControlsLineHidesInterruptWithoutActiveTurn(t *testing.T) {
	m := newComposeInterruptTestModel("codex")

	line := m.composeControlsLine()
	if strings.Contains(line, "Interrupt") {
		t.Fatalf("did not expect interrupt control without active turn, got %q", line)
	}
	if _, ok := findComposeSpan(m.composeControlSpans(), composeControlActionInterruptTurn); ok {
		t.Fatalf("did not expect interrupt span without active turn")
	}
}

func TestComposeControlsLineShowsInterruptForActiveRequest(t *testing.T) {
	m := newComposeInterruptTestModel("codex")
	m.startRequestActivity("s1", "codex")

	line := m.composeControlsLine()
	if !strings.Contains(line, "[Interrupt]") {
		t.Fatalf("expected interrupt control in compose footer, got %q", line)
	}
	span, ok := findComposeSpan(m.composeControlSpans(), composeControlActionInterruptTurn)
	if !ok {
		t.Fatalf("expected interrupt control span")
	}
	if span.start < 0 || span.end < span.start || span.end >= len(line) {
		t.Fatalf("invalid interrupt span %#v for line length %d", span, len(line))
	}
	if got := line[span.start : span.end+1]; got != "[Interrupt]" {
		t.Fatalf("expected interrupt span text, got %q", got)
	}
}

func TestComposeControlsLineHidesInterruptForProviderWithoutSupport(t *testing.T) {
	m := newComposeInterruptTestModel("claude")
	m.startRequestActivity("s1", "claude")

	line := m.composeControlsLine()
	if strings.Contains(line, "Interrupt") {
		t.Fatalf("did not expect interrupt control for provider without interrupt support, got %q", line)
	}
}

func TestComposeControlsLineShowsInterruptingWhileRequestInFlight(t *testing.T) {
	m := newComposeInterruptTestModel("codex")
	m.composeInterruptInFlightSessionID = "s1"

	line := m.composeControlsLine()
	if !strings.Contains(line, "[Interrupting...]") {
		t.Fatalf("expected in-flight interrupt label, got %q", line)
	}
}

func TestRequestComposeInterruptCmdStartsInFlightAndScope(t *testing.T) {
	m := newComposeInterruptTestModel("codex")
	m.startRequestActivity("s1", "codex")

	cmd := m.requestComposeInterruptCmd()
	if cmd == nil {
		t.Fatalf("expected interrupt command")
	}
	if got := m.composeInterruptInFlightSessionID; got != "s1" {
		t.Fatalf("expected in-flight session s1, got %q", got)
	}
	if !m.hasRequestScope(requestScopeSessionInterrupt) {
		t.Fatalf("expected session interrupt request scope")
	}

	duplicate := m.requestComposeInterruptCmd()
	if duplicate != nil {
		t.Fatalf("expected duplicate interrupt request to be ignored")
	}
	if got := m.status; got != "interrupt already in progress" {
		t.Fatalf("unexpected duplicate interrupt status: %q", got)
	}
}

func TestMouseComposeInterruptClickQueuesInterruptCommand(t *testing.T) {
	m := newComposeInterruptTestModel("codex")
	m.startRequestActivity("s1", "codex")
	line := m.composeControlsLine()
	if line == "" {
		t.Fatalf("expected compose controls line")
	}
	span, ok := findComposeSpan(m.composeControlSpans(), composeControlActionInterruptTurn)
	if !ok {
		t.Fatalf("expected interrupt span in compose controls")
	}
	layout := m.resolveMouseLayout()
	y := m.composeControlsRow()
	x := layout.rightStart + span.start

	handled := m.reduceComposeControlsLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y}, layout)
	if !handled {
		t.Fatalf("expected compose interrupt click to be handled")
	}
	if m.pendingMouseCmd == nil {
		t.Fatalf("expected interrupt click to queue a command")
	}
	if got := m.composeInterruptInFlightSessionID; got != "s1" {
		t.Fatalf("expected in-flight session s1, got %q", got)
	}
	if !m.hasRequestScope(requestScopeSessionInterrupt) {
		t.Fatalf("expected interrupt request scope after click")
	}
}

func TestReduceMutationMessagesInterruptSuccessClearsInFlightAndRunningState(t *testing.T) {
	m := newComposeInterruptTestModel("codex")
	m.startRequestActivity("s1", "codex")
	m.recents.StartRun("s1", "turn-1", time.Now().UTC())
	m.recentsCompletionWatching["s1"] = "turn-1"
	recentsCtx := m.replaceRequestScope(recentsRequestScopeName("s1"))
	interruptCtx := m.replaceRequestScope(requestScopeSessionInterrupt)
	m.composeInterruptInFlightSessionID = "s1"

	nextModel, cmd := m.Update(interruptMsg{id: "s1"})
	next, ok := nextModel.(*Model)
	if !ok || next == nil {
		t.Fatalf("expected updated model from interrupt message, got %T", nextModel)
	}
	m = next
	if cmd != nil {
		t.Fatalf("did not expect follow-up command on interrupt success")
	}
	if got := m.status; got != "interrupt sent" {
		t.Fatalf("unexpected status %q", got)
	}
	if m.requestActivity.active {
		t.Fatalf("expected request activity to stop after interrupt")
	}
	if m.recents.IsRunning("s1") {
		t.Fatalf("expected recents running state to clear after interrupt")
	}
	if _, ok := m.recentsCompletionWatching["s1"]; ok {
		t.Fatalf("expected recents completion watch to clear after interrupt")
	}
	if got := m.composeInterruptInFlightSessionID; got != "" {
		t.Fatalf("expected in-flight interrupt to clear, got %q", got)
	}
	if m.hasRequestScope(requestScopeSessionInterrupt) {
		t.Fatalf("expected session interrupt scope to be removed")
	}
	select {
	case <-recentsCtx.Done():
	default:
		t.Fatalf("expected recents watch scope to be canceled")
	}
	select {
	case <-interruptCtx.Done():
	default:
		t.Fatalf("expected interrupt scope to be canceled")
	}
}

func TestReduceMutationMessagesInterruptErrorClearsInFlightOnly(t *testing.T) {
	m := newComposeInterruptTestModel("codex")
	m.recents.StartRun("s1", "turn-1", time.Now().UTC())
	interruptCtx := m.replaceRequestScope(requestScopeSessionInterrupt)
	m.composeInterruptInFlightSessionID = "s1"

	nextModel, cmd := m.Update(interruptMsg{id: "s1", err: errors.New("boom")})
	next, ok := nextModel.(*Model)
	if !ok || next == nil {
		t.Fatalf("expected updated model from interrupt error, got %T", nextModel)
	}
	m = next
	if cmd != nil {
		t.Fatalf("did not expect follow-up command on interrupt error")
	}
	if !strings.Contains(m.status, "interrupt error: boom") {
		t.Fatalf("unexpected error status %q", m.status)
	}
	if !m.recents.IsRunning("s1") {
		t.Fatalf("expected recents running state to remain on interrupt error")
	}
	if got := m.composeInterruptInFlightSessionID; got != "" {
		t.Fatalf("expected in-flight interrupt to clear, got %q", got)
	}
	if m.hasRequestScope(requestScopeSessionInterrupt) {
		t.Fatalf("expected session interrupt scope to be removed")
	}
	select {
	case <-interruptCtx.Done():
	default:
		t.Fatalf("expected interrupt scope to be canceled")
	}
}

func TestRequestComposeInterruptCmdRequiresSelectedComposeSession(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeCompose

	cmd := m.requestComposeInterruptCmd()
	if cmd != nil {
		t.Fatalf("expected no interrupt command without selected session")
	}
	if got := m.status; got != "select a session to interrupt" {
		t.Fatalf("unexpected status %q", got)
	}
}

func TestRequestComposeInterruptCmdRejectsSessionWithoutInterruptSignal(t *testing.T) {
	m := newComposeInterruptTestModel("codex")

	cmd := m.requestComposeInterruptCmd()
	if cmd != nil {
		t.Fatalf("expected no interrupt command without active interrupt signal")
	}
	if got := m.status; got != "no interruptible turn in this session" {
		t.Fatalf("unexpected status %q", got)
	}
}

func TestComposeSessionSupportsInterruptRejectsBlankAndUnknownSession(t *testing.T) {
	m := newComposeInterruptTestModel("codex")
	if m.composeSessionSupportsInterrupt("  ") {
		t.Fatalf("expected blank session id to be unsupported")
	}
	if m.composeSessionSupportsInterrupt("missing") {
		t.Fatalf("expected unknown session id to be unsupported")
	}
}

func TestCanInterruptComposeSessionRejectsBlankAndUnknownSession(t *testing.T) {
	m := newComposeInterruptTestModel("codex")
	if m.canInterruptComposeSession("  ") {
		t.Fatalf("expected blank session id to be non-interruptible")
	}
	if m.canInterruptComposeSession("missing") {
		t.Fatalf("expected unknown session id to be non-interruptible")
	}
}

func TestComposeSessionHasInterruptSignalNilModelReturnsFalse(t *testing.T) {
	var m *Model
	if m.composeSessionHasInterruptSignal("s1") {
		t.Fatalf("expected nil model to have no interrupt signal")
	}
}

func TestClearComposeInterruptRequestIgnoresMismatchedSessionID(t *testing.T) {
	m := newComposeInterruptTestModel("codex")
	m.composeInterruptInFlightSessionID = "s1"
	_ = m.replaceRequestScope(requestScopeSessionInterrupt)

	m.clearComposeInterruptRequest("s2")
	if got := m.composeInterruptInFlightSessionID; got != "s1" {
		t.Fatalf("expected in-flight session to remain unchanged, got %q", got)
	}
	if !m.hasRequestScope(requestScopeSessionInterrupt) {
		t.Fatalf("expected interrupt scope to remain active for mismatched clear")
	}
}
