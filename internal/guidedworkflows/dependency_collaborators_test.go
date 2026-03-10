package guidedworkflows

import (
	"context"
	"testing"
	"time"
)

type dependencyValidatorStub struct {
	calls         int
	lastRunID     string
	lastDependsOn []string
	result        []RunDependency
	err           error
}

func (s *dependencyValidatorStub) NormalizeAndValidate(
	runID string,
	dependsOnRunIDs []string,
	_ map[string]*WorkflowRun,
) ([]RunDependency, error) {
	s.calls++
	s.lastRunID = runID
	s.lastDependsOn = append([]string(nil), dependsOnRunIDs...)
	if s.err != nil {
		return nil, s.err
	}
	return append([]RunDependency(nil), s.result...), nil
}

type dependencyGraphIndexStub struct {
	setRunIDs []string
}

func (s *dependencyGraphIndexStub) SetRun(run *WorkflowRun) {
	if run == nil {
		return
	}
	s.setRunIDs = append(s.setRunIDs, run.ID)
}

func (s *dependencyGraphIndexStub) RemoveRun(string) {}

func (s *dependencyGraphIndexStub) Dependents(string) []string {
	return nil
}

func (s *dependencyGraphIndexStub) Reset() {}

type dependencyEvaluatorStub struct {
	calls  int
	states []RunDependencyState
}

func (s *dependencyEvaluatorStub) Evaluate(
	now time.Time,
	_ *WorkflowRun,
	_ func(runID string) (*WorkflowRun, bool),
) RunDependencyState {
	s.calls++
	if len(s.states) == 0 {
		return RunDependencyState{Ready: true}
	}
	idx := s.calls - 1
	if idx >= len(s.states) {
		idx = len(s.states) - 1
	}
	out := s.states[idx]
	if out.LastEvaluatedAt == nil && !now.IsZero() {
		at := now.UTC()
		out.LastEvaluatedAt = &at
	}
	return out
}

type queuedRunActivatorStub struct {
	calls       int
	activateNow bool
}

func (s *queuedRunActivatorStub) ShouldActivate(_ *WorkflowRun, _ RunDependencyState) bool {
	s.calls++
	return s.activateNow
}

func TestRunServiceDependencyCollaboratorOptionsApply(t *testing.T) {
	t.Parallel()

	service := &InMemoryRunService{}
	validator := &dependencyValidatorStub{}
	graph := &dependencyGraphIndexStub{}
	evaluator := &dependencyEvaluatorStub{}
	activator := &queuedRunActivatorStub{}

	WithDependencyValidator(validator)(service)
	WithDependencyGraphIndex(graph)(service)
	WithDependencyEvaluator(evaluator)(service)
	WithQueuedRunActivator(activator)(service)

	if got, ok := service.dependencyValidator.(*dependencyValidatorStub); !ok || got != validator {
		t.Fatalf("expected dependency validator option to apply")
	}
	if got, ok := service.dependencyGraph.(*dependencyGraphIndexStub); !ok || got != graph {
		t.Fatalf("expected dependency graph option to apply")
	}
	if got, ok := service.dependencyEvaluator.(*dependencyEvaluatorStub); !ok || got != evaluator {
		t.Fatalf("expected dependency evaluator option to apply")
	}
	if got, ok := service.queuedRunActivator.(*queuedRunActivatorStub); !ok || got != activator {
		t.Fatalf("expected queued run activator option to apply")
	}
}

func TestRunServiceUsesInjectedDependencyValidatorDuringCreate(t *testing.T) {
	t.Parallel()

	validator := &dependencyValidatorStub{
		result: []RunDependency{{RunID: "gwf-any", Condition: DependencyConditionOnCompleted}},
	}
	service := NewRunService(
		Config{Enabled: true},
		WithDependencyValidator(validator),
	)
	t.Cleanup(service.Close)

	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID:     "ws-1",
		DependsOnRunIDs: []string{"gwf-any"},
	})
	if err != nil {
		t.Fatalf("CreateRun returned error: %v", err)
	}
	if validator.calls == 0 {
		t.Fatalf("expected injected dependency validator to be invoked")
	}
	if len(run.Dependencies) != 1 || run.Dependencies[0].RunID != "gwf-any" {
		t.Fatalf("expected injected dependency list to be persisted, got %+v", run.Dependencies)
	}
}

func TestRunServiceUsesInjectedDependencyGraphIndexDuringCreate(t *testing.T) {
	t.Parallel()

	graph := &dependencyGraphIndexStub{}
	service := NewRunService(
		Config{Enabled: true},
		WithDependencyGraphIndex(graph),
	)
	t.Cleanup(service.Close)

	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
	})
	if err != nil {
		t.Fatalf("CreateRun returned error: %v", err)
	}
	if len(graph.setRunIDs) == 0 || graph.setRunIDs[0] != run.ID {
		t.Fatalf("expected injected dependency graph index to record run %q, got %#v", run.ID, graph.setRunIDs)
	}
}

func TestRunServiceUsesInjectedQueuedRunActivatorForQueuedRuns(t *testing.T) {
	t.Parallel()

	evaluator := &dependencyEvaluatorStub{
		states: []RunDependencyState{
			{Ready: false, Reason: "waiting on stub dependency"},
			{Ready: false, Reason: "waiting on stub dependency"},
			{Ready: true},
		},
	}
	activator := &queuedRunActivatorStub{activateNow: false}
	service := NewRunService(
		Config{Enabled: true},
		WithDependencyEvaluator(evaluator),
		WithQueuedRunActivator(activator),
	)
	t.Cleanup(service.Close)

	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
	})
	if err != nil {
		t.Fatalf("CreateRun returned error: %v", err)
	}
	queued, err := service.StartRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	if queued.Status != WorkflowRunStatusQueued {
		t.Fatalf("expected run to be queued, got %q", queued.Status)
	}

	if err := service.advanceViaQueue(context.Background(), run.ID, "dependency_changed"); err != nil {
		t.Fatalf("advanceViaQueue returned error: %v", err)
	}
	current, err := service.GetRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("GetRun returned error: %v", err)
	}
	if current.Status != WorkflowRunStatusQueued {
		t.Fatalf("expected queued run to remain queued when activator denies activation, got %q", current.Status)
	}
	if activator.calls == 0 {
		t.Fatalf("expected injected queued run activator to be invoked")
	}
}

func TestRunServiceDependencyGraphOrDefaultReturnsFallback(t *testing.T) {
	t.Parallel()

	var nilService *InMemoryRunService
	if got := nilService.dependencyGraphOrDefault(); got == nil {
		t.Fatalf("expected non-nil fallback dependency graph index")
	}

	graph := &dependencyGraphIndexStub{}
	service := &InMemoryRunService{dependencyGraph: graph}
	if got := service.dependencyGraphOrDefault(); got != graph {
		t.Fatalf("expected configured dependency graph index to be returned")
	}
}
