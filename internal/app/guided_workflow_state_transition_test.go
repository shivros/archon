package app

import (
	"strings"
	"testing"
	"time"

	"control/internal/guidedworkflows"
	"control/internal/types"
)

type fixedGuidedWorkflowReflowPolicy struct {
	shouldReflow bool
	calls        int
	lastInput    GuidedWorkflowReflowInput
}

func (p *fixedGuidedWorkflowReflowPolicy) ShouldReflow(input GuidedWorkflowReflowInput) bool {
	p.calls++
	p.lastInput = input
	return p.shouldReflow
}

type stubGuidedWorkflowStateTransitionGateway struct {
	applyRunCalls             int
	applySnapshotCalls        int
	applyPreviewCalls         int
	applyPreviewSnapshotCalls int
}

func (g *stubGuidedWorkflowStateTransitionGateway) ApplyRun(*guidedworkflows.WorkflowRun) {
	g.applyRunCalls++
}

func (g *stubGuidedWorkflowStateTransitionGateway) ApplySnapshot(*guidedworkflows.WorkflowRun, []guidedworkflows.RunTimelineEvent) {
	g.applySnapshotCalls++
}

func (g *stubGuidedWorkflowStateTransitionGateway) ApplyPreview(*guidedworkflows.WorkflowRun) {
	g.applyPreviewCalls++
}

func (g *stubGuidedWorkflowStateTransitionGateway) ApplyPreviewSnapshot(*guidedworkflows.WorkflowRun, []guidedworkflows.RunTimelineEvent) {
	g.applyPreviewSnapshotCalls++
}

type stubGuidedWorkflowInteractiveStateTransitionGateway struct {
	applyRunCalls      int
	applySnapshotCalls int
}

func (g *stubGuidedWorkflowInteractiveStateTransitionGateway) ApplyRun(*guidedworkflows.WorkflowRun) {
	g.applyRunCalls++
}

func (g *stubGuidedWorkflowInteractiveStateTransitionGateway) ApplySnapshot(*guidedworkflows.WorkflowRun, []guidedworkflows.RunTimelineEvent) {
	g.applySnapshotCalls++
}

type stubGuidedWorkflowPreviewStateTransitionGateway struct {
	applyPreviewCalls         int
	applyPreviewSnapshotCalls int
}

func (g *stubGuidedWorkflowPreviewStateTransitionGateway) ApplyPreview(*guidedworkflows.WorkflowRun) {
	g.applyPreviewCalls++
}

func (g *stubGuidedWorkflowPreviewStateTransitionGateway) ApplyPreviewSnapshot(*guidedworkflows.WorkflowRun, []guidedworkflows.RunTimelineEvent) {
	g.applyPreviewSnapshotCalls++
}

func TestWithGuidedWorkflowReflowPolicyConfiguresAndResetsDefault(t *testing.T) {
	custom := &fixedGuidedWorkflowReflowPolicy{shouldReflow: true}
	m := NewModel(nil, WithGuidedWorkflowReflowPolicy(custom))
	if got := m.guidedWorkflowReflowPolicyOrDefault(); got != custom {
		t.Fatalf("expected custom guided workflow reflow policy, got %T", got)
	}

	WithGuidedWorkflowReflowPolicy(nil)(&m)
	if _, ok := m.guidedWorkflowReflowPolicyOrDefault().(defaultGuidedWorkflowReflowPolicy); !ok {
		t.Fatalf("expected default guided workflow reflow policy after reset")
	}
}

func TestWithGuidedWorkflowReflowPolicyNilModelNoop(t *testing.T) {
	var m *Model
	WithGuidedWorkflowReflowPolicy(&fixedGuidedWorkflowReflowPolicy{shouldReflow: true})(m)
}

func TestGuidedWorkflowReflowPolicyOrDefaultNilModelUsesDefault(t *testing.T) {
	var m *Model
	if _, ok := m.guidedWorkflowReflowPolicyOrDefault().(defaultGuidedWorkflowReflowPolicy); !ok {
		t.Fatalf("expected nil model to return default guided workflow reflow policy")
	}
}

func TestWithGuidedWorkflowStateTransitionGatewayConfiguresAndResetsDefault(t *testing.T) {
	custom := &stubGuidedWorkflowStateTransitionGateway{}
	m := NewModel(nil, WithGuidedWorkflowStateTransitionGateway(custom))
	if got := m.guidedWorkflowInteractiveStateTransitionGatewayOrDefault(); got != custom {
		t.Fatalf("expected custom interactive gateway, got %T", got)
	}
	if got := m.guidedWorkflowPreviewStateTransitionGatewayOrDefault(); got != custom {
		t.Fatalf("expected custom preview gateway, got %T", got)
	}

	WithGuidedWorkflowStateTransitionGateway(nil)(&m)
	if _, ok := m.guidedWorkflowInteractiveStateTransitionGatewayOrDefault().(defaultGuidedWorkflowStateTransitionGateway); !ok {
		t.Fatalf("expected default interactive gateway after reset")
	}
	if _, ok := m.guidedWorkflowPreviewStateTransitionGatewayOrDefault().(defaultGuidedWorkflowStateTransitionGateway); !ok {
		t.Fatalf("expected default preview gateway after reset")
	}
}

func TestWithGuidedWorkflowStateTransitionGatewayNilModelNoop(t *testing.T) {
	var m *Model
	WithGuidedWorkflowStateTransitionGateway(&stubGuidedWorkflowStateTransitionGateway{})(m)
}

func TestWithGuidedWorkflowInteractiveStateTransitionGatewayConfiguresAndResetsDefault(t *testing.T) {
	custom := &stubGuidedWorkflowInteractiveStateTransitionGateway{}
	m := NewModel(nil, WithGuidedWorkflowInteractiveStateTransitionGateway(custom))
	if got := m.guidedWorkflowInteractiveStateTransitionGatewayOrDefault(); got != custom {
		t.Fatalf("expected custom interactive gateway, got %T", got)
	}
	if _, ok := m.guidedWorkflowPreviewStateTransitionGatewayOrDefault().(defaultGuidedWorkflowStateTransitionGateway); !ok {
		t.Fatalf("expected preview gateway to remain default")
	}

	WithGuidedWorkflowInteractiveStateTransitionGateway(nil)(&m)
	if _, ok := m.guidedWorkflowInteractiveStateTransitionGatewayOrDefault().(defaultGuidedWorkflowStateTransitionGateway); !ok {
		t.Fatalf("expected default interactive gateway after reset")
	}
}

func TestWithGuidedWorkflowPreviewStateTransitionGatewayConfiguresAndResetsDefault(t *testing.T) {
	custom := &stubGuidedWorkflowPreviewStateTransitionGateway{}
	m := NewModel(nil, WithGuidedWorkflowPreviewStateTransitionGateway(custom))
	if got := m.guidedWorkflowPreviewStateTransitionGatewayOrDefault(); got != custom {
		t.Fatalf("expected custom preview gateway, got %T", got)
	}
	if _, ok := m.guidedWorkflowInteractiveStateTransitionGatewayOrDefault().(defaultGuidedWorkflowStateTransitionGateway); !ok {
		t.Fatalf("expected interactive gateway to remain default")
	}

	WithGuidedWorkflowPreviewStateTransitionGateway(nil)(&m)
	if _, ok := m.guidedWorkflowPreviewStateTransitionGatewayOrDefault().(defaultGuidedWorkflowStateTransitionGateway); !ok {
		t.Fatalf("expected default preview gateway after reset")
	}
}

func TestWithGuidedWorkflowInteractiveStateTransitionGatewayNilModelNoop(t *testing.T) {
	var m *Model
	WithGuidedWorkflowInteractiveStateTransitionGateway(&stubGuidedWorkflowInteractiveStateTransitionGateway{})(m)
}

func TestWithGuidedWorkflowPreviewStateTransitionGatewayNilModelNoop(t *testing.T) {
	var m *Model
	WithGuidedWorkflowPreviewStateTransitionGateway(&stubGuidedWorkflowPreviewStateTransitionGateway{})(m)
}

func TestDefaultGuidedWorkflowStateTransitionGatewayApplyRunUsesReflowPolicy(t *testing.T) {
	now := time.Date(2026, 2, 23, 10, 0, 0, 0, time.UTC)
	running := newWorkflowRunFixture("gwf-transition-policy", guidedworkflows.WorkflowRunStatusRunning, now)
	failed := newWorkflowRunFixture("gwf-transition-policy", guidedworkflows.WorkflowRunStatusFailed, now.Add(30*time.Second))
	failed.LastError = "workflow run interrupted by daemon restart"

	policy := &fixedGuidedWorkflowReflowPolicy{shouldReflow: true}
	m := NewModel(nil, WithGuidedWorkflowReflowPolicy(policy))
	enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{workspaceID: "ws1", worktreeID: "wt1", sessionID: "s1"})
	m.resize(100, 24)

	gateway := m.guidedWorkflowInteractiveStateTransitionGatewayOrDefault()
	gateway.ApplyRun(running)
	liveViewportHeight := m.viewport.Height()
	policy.calls = 0

	gateway.ApplyRun(failed)
	if policy.calls != 1 {
		t.Fatalf("expected one reflow policy call for failed transition, got %d", policy.calls)
	}
	if policy.lastInput.BeforeInputLines == policy.lastInput.AfterInputLines {
		t.Fatalf("expected failed transition to change input line count")
	}
	if got := m.viewport.Height(); got >= liveViewportHeight {
		t.Fatalf("expected reflowed failed summary viewport to shrink: live=%d summary=%d", liveViewportHeight, got)
	}
}

func TestDefaultGuidedWorkflowStateTransitionGatewayApplyRunRespectsPolicyBlock(t *testing.T) {
	now := time.Date(2026, 2, 23, 10, 5, 0, 0, time.UTC)
	running := newWorkflowRunFixture("gwf-transition-policy-block", guidedworkflows.WorkflowRunStatusRunning, now)
	failed := newWorkflowRunFixture("gwf-transition-policy-block", guidedworkflows.WorkflowRunStatusFailed, now.Add(30*time.Second))
	failed.LastError = "workflow run interrupted by daemon restart"

	policy := &fixedGuidedWorkflowReflowPolicy{shouldReflow: false}
	m := NewModel(nil, WithGuidedWorkflowReflowPolicy(policy))
	enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{workspaceID: "ws1", worktreeID: "wt1", sessionID: "s1"})
	m.resize(100, 24)

	gateway := m.guidedWorkflowInteractiveStateTransitionGatewayOrDefault()
	gateway.ApplyRun(running)
	liveViewportHeight := m.viewport.Height()
	policy.calls = 0

	gateway.ApplyRun(failed)
	if policy.calls != 1 {
		t.Fatalf("expected one reflow policy call for failed transition, got %d", policy.calls)
	}
	if got := m.viewport.Height(); got != liveViewportHeight {
		t.Fatalf("expected blocked reflow to keep viewport height %d, got %d", liveViewportHeight, got)
	}
}

func TestDefaultGuidedWorkflowStateTransitionGatewayApplyPreviewNilModelNoop(t *testing.T) {
	gateway := NewDefaultGuidedWorkflowStateTransitionGateway(nil)
	gateway.ApplyPreview(&guidedworkflows.WorkflowRun{ID: "gwf-preview"})
}

func TestDefaultGuidedWorkflowStateTransitionGatewayApplyPreviewNilControllerNoop(t *testing.T) {
	m := NewModel(nil)
	m.guidedWorkflow = nil
	gateway := NewDefaultGuidedWorkflowStateTransitionGateway(&m)
	gateway.ApplyPreview(&guidedworkflows.WorkflowRun{ID: "gwf-preview"})
}

func TestDefaultGuidedWorkflowStateTransitionGatewayApplyPreviewUpdatesController(t *testing.T) {
	m := NewModel(nil)
	gateway := NewDefaultGuidedWorkflowStateTransitionGateway(&m)
	run := &guidedworkflows.WorkflowRun{ID: "gwf-preview", Status: guidedworkflows.WorkflowRunStatusRunning}
	gateway.ApplyPreview(run)
	if m.guidedWorkflow == nil || m.guidedWorkflow.RunID() != "gwf-preview" {
		t.Fatalf("expected preview transition to set guided workflow run id")
	}
}

func TestDefaultGuidedWorkflowStateTransitionGatewayApplySnapshotUpdatesController(t *testing.T) {
	now := time.Now().UTC()
	run := newWorkflowRunFixture("gwf-snapshot-apply", guidedworkflows.WorkflowRunStatusRunning, now)
	timeline := []guidedworkflows.RunTimelineEvent{
		{At: now, Type: "run_started", RunID: run.ID},
		{At: now.Add(2 * time.Second), Type: "step_completed", RunID: run.ID},
	}
	m := NewModel(nil)
	gateway := NewDefaultGuidedWorkflowStateTransitionGateway(&m)

	gateway.ApplySnapshot(run, timeline)
	if m.guidedWorkflow == nil || m.guidedWorkflow.RunID() != run.ID {
		t.Fatalf("expected snapshot transition to set guided workflow run id")
	}
	if got := len(m.guidedWorkflow.timeline); got != len(timeline) {
		t.Fatalf("expected snapshot transition to set timeline length %d, got %d", len(timeline), got)
	}
}

func TestDefaultGuidedWorkflowStateTransitionGatewayApplyPreviewSnapshotRendersPassivePreview(t *testing.T) {
	now := time.Now().UTC()
	run := newWorkflowRunFixture("gwf-preview-snapshot", guidedworkflows.WorkflowRunStatusRunning, now)
	m := newPhase0ModelWithSession("codex")
	m.workflowRuns = []*guidedworkflows.WorkflowRun{run}
	m.sessionMeta["s1"] = &types.SessionMeta{
		SessionID:     "s1",
		WorkspaceID:   "ws1",
		WorkflowRunID: run.ID,
	}
	m.applySidebarItems()
	if m.sidebar == nil || !m.sidebar.SelectByWorkflowID(run.ID) {
		t.Fatalf("expected workflow to be selectable")
	}
	_ = m.onSelectionChangedImmediate()

	gateway := m.guidedWorkflowPreviewStateTransitionGatewayOrDefault()
	gateway.ApplyPreviewSnapshot(run, []guidedworkflows.RunTimelineEvent{
		{At: now, Type: "run_started", RunID: run.ID},
	})
	if strings.Contains(strings.ToLower(m.contentRaw), "no events yet") {
		t.Fatalf("expected passive preview render to include snapshot timeline, got %q", m.contentRaw)
	}
}

func TestDefaultGuidedWorkflowStateTransitionGatewayApplyNoUpdateStillEvaluatesPolicy(t *testing.T) {
	policy := &fixedGuidedWorkflowReflowPolicy{shouldReflow: false}
	m := NewModel(nil, WithGuidedWorkflowReflowPolicy(policy))
	enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{workspaceID: "ws1", worktreeID: "wt1", sessionID: "s1"})
	m.resize(100, 24)
	gateway := defaultGuidedWorkflowStateTransitionGateway{model: &m}

	gateway.apply(nil)
	if policy.calls != 1 {
		t.Fatalf("expected apply(nil) to evaluate reflow policy once, got %d", policy.calls)
	}
}

func TestDefaultGuidedWorkflowStateTransitionGatewayApplyNoopWithoutModelOrController(t *testing.T) {
	defaultGuidedWorkflowStateTransitionGateway{}.apply(func(controller *GuidedWorkflowUIController) {
		t.Fatalf("did not expect update callback for nil model")
	})

	m := NewModel(nil)
	m.guidedWorkflow = nil
	defaultGuidedWorkflowStateTransitionGateway{model: &m}.apply(func(controller *GuidedWorkflowUIController) {
		t.Fatalf("did not expect update callback for nil guided workflow controller")
	})
}
