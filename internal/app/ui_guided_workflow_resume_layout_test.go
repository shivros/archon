package app

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"control/internal/guidedworkflows"
)

func TestUIHarnessGuidedWorkflowFailedSummaryResumeInputStaysVisible(t *testing.T) {
	now := time.Date(2026, 2, 24, 9, 20, 0, 0, time.UTC)
	failed := newWorkflowRunFixture("gwf-ui-harness-failed", guidedworkflows.WorkflowRunStatusFailed, now)
	failed.LastError = "workflow run interrupted by daemon restart"
	completedAt := now.Add(5 * time.Second)
	failed.CompletedAt = &completedAt

	model := newPhase0ModelWithSession("codex")
	h := newUIHarness(t, &model)
	defer h.Close()

	h.Resize(80, 24)
	enterGuidedWorkflowForTest(h.model, guidedWorkflowLaunchContext{workspaceID: "ws1", worktreeID: "wt1", sessionID: "s1"})
	h.model.guidedWorkflow.SetRun(newWorkflowRunFixture(failed.ID, guidedworkflows.WorkflowRunStatusRunning, now.Add(-time.Second)))
	h.apply(workflowRunSnapshotMsg{run: failed})

	if h.model.mode != uiModeGuidedWorkflow {
		t.Fatalf("expected guided workflow mode")
	}
	if h.model.guidedWorkflow == nil || !h.model.guidedWorkflow.CanResumeFailedRun() {
		t.Fatalf("expected failed summary to allow resume")
	}

	longResume := strings.Repeat("Continue from the last successful step and verify all follow-up checks. ", 3)
	h.apply(tea.PasteMsg{Content: longResume})
	h.Resize(64, 18)

	if h.model.guidedWorkflowResumeInput == nil || !strings.Contains(h.model.guidedWorkflowResumeInput.Value(), "Continue from the last successful step") {
		t.Fatalf("expected pasted resume text to remain in resume input")
	}

	visibleLines := 1 + h.model.viewport.Height() + 1 + h.model.modeInputLineCount()
	maxContentLines := h.model.height - 1
	if visibleLines > maxContentLines {
		t.Fatalf("expected guided workflow summary layout to fit viewport; visible=%d max=%d", visibleLines, maxContentLines)
	}
}
