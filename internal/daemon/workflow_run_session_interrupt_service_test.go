package daemon

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"control/internal/guidedworkflows"
	"control/internal/types"
)

func TestWorkflowRunSessionInterruptServiceInterruptsActiveSessions(t *testing.T) {
	gateway := &stubWorkflowRunSessionInterruptGateway{
		sessions: map[string]*types.Session{
			"sess-run":     {ID: "sess-run", Status: types.SessionStatusRunning},
			"sess-step":    {ID: "sess-step", Status: types.SessionStatusInactive},
			"sess-attempt": {ID: "sess-attempt", Status: types.SessionStatusCreated},
			"sess-meta":    {ID: "sess-meta", Status: types.SessionStatusStarting},
		},
	}
	meta := &stubWorkflowRunSessionInterruptMetaStore{
		list: []*types.SessionMeta{
			{SessionID: "sess-meta", WorkflowRunID: "gwf-1"},
			{SessionID: "sess-other", WorkflowRunID: "gwf-other"},
		},
	}
	service := &workflowRunSessionInterruptService{
		resolver: &workflowRunSessionTargetResolverService{
			sessionMeta: meta,
		},
		executor: &workflowRunSessionInterruptExecutorService{
			sessions: gateway,
			timeout:  0,
		},
	}
	run := &guidedworkflows.WorkflowRun{
		ID:        "gwf-1",
		SessionID: "sess-run",
		Phases: []guidedworkflows.PhaseRun{
			{
				Steps: []guidedworkflows.StepRun{
					{
						Execution: &guidedworkflows.StepExecutionRef{
							SessionID: "sess-step",
						},
						ExecutionAttempts: []guidedworkflows.StepExecutionRef{
							{SessionID: "sess-attempt"},
							{SessionID: "sess-run"},
						},
					},
				},
			},
		},
	}

	err := service.InterruptWorkflowRunSessions(context.Background(), run)
	if err != nil {
		t.Fatalf("unexpected interrupt error: %v", err)
	}
	if !reflect.DeepEqual(gateway.getCalls, []string{"sess-attempt", "sess-meta", "sess-run", "sess-step"}) {
		t.Fatalf("unexpected lookup order: %#v", gateway.getCalls)
	}
	if !reflect.DeepEqual(gateway.interruptCalls, []string{"sess-attempt", "sess-meta", "sess-run"}) {
		t.Fatalf("unexpected interrupted sessions: %#v", gateway.interruptCalls)
	}
}

func TestWorkflowRunSessionInterruptServiceBestEffortErrors(t *testing.T) {
	lookupErr := errors.New("lookup failed")
	interruptErr := errors.New("interrupt failed")
	gateway := &stubWorkflowRunSessionInterruptGateway{
		sessions: map[string]*types.Session{
			"sess-active":       {ID: "sess-active", Status: types.SessionStatusCreated},
			"sess-interrupterr": {ID: "sess-interrupterr", Status: types.SessionStatusRunning},
		},
		getErrByID: map[string]error{
			"sess-missing": ErrSessionNotFound,
			"sess-geterr":  lookupErr,
		},
		interruptErrByID: map[string]error{
			"sess-interrupterr": interruptErr,
		},
	}
	meta := &stubWorkflowRunSessionInterruptMetaStore{
		list: []*types.SessionMeta{
			{SessionID: "sess-active", WorkflowRunID: "gwf-1"},
		},
	}
	service := &workflowRunSessionInterruptService{
		resolver: &workflowRunSessionTargetResolverService{
			sessionMeta: meta,
		},
		executor: &workflowRunSessionInterruptExecutorService{
			sessions: gateway,
			timeout:  0,
		},
	}
	run := &guidedworkflows.WorkflowRun{
		ID:        "gwf-1",
		SessionID: "sess-interrupterr",
		Phases: []guidedworkflows.PhaseRun{
			{
				Steps: []guidedworkflows.StepRun{
					{
						Execution: &guidedworkflows.StepExecutionRef{SessionID: "sess-missing"},
						ExecutionAttempts: []guidedworkflows.StepExecutionRef{
							{SessionID: "sess-geterr"},
						},
					},
				},
			},
		},
	}

	err := service.InterruptWorkflowRunSessions(context.Background(), run)
	if err == nil {
		t.Fatalf("expected joined interrupt error")
	}
	if !errors.Is(err, lookupErr) {
		t.Fatalf("expected lookup error in joined result, got %v", err)
	}
	if !errors.Is(err, interruptErr) {
		t.Fatalf("expected interrupt error in joined result, got %v", err)
	}
	if errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("did not expect not-found lookup to fail interrupt, got %v", err)
	}
	if !reflect.DeepEqual(gateway.interruptCalls, []string{"sess-active", "sess-interrupterr"}) {
		t.Fatalf("unexpected interrupted sessions: %#v", gateway.interruptCalls)
	}
}

func TestWorkflowRunSessionTargetResolverCollectsUniqueSessionIDs(t *testing.T) {
	meta := &stubWorkflowRunSessionInterruptMetaStore{
		list: []*types.SessionMeta{
			{SessionID: "sess-meta", WorkflowRunID: "gwf-1"},
			{SessionID: "sess-meta", WorkflowRunID: "gwf-1"},
			{SessionID: "sess-other", WorkflowRunID: "gwf-2"},
		},
	}
	resolver := &workflowRunSessionTargetResolverService{sessionMeta: meta}
	run := &guidedworkflows.WorkflowRun{
		ID:        "gwf-1",
		SessionID: "sess-run",
		Phases: []guidedworkflows.PhaseRun{
			{
				Steps: []guidedworkflows.StepRun{
					{
						Execution: &guidedworkflows.StepExecutionRef{SessionID: "sess-step"},
						ExecutionAttempts: []guidedworkflows.StepExecutionRef{
							{SessionID: "sess-run"},
							{SessionID: "sess-attempt"},
						},
					},
				},
			},
		},
	}

	sessionIDs := resolver.ResolveWorkflowRunSessionIDs(context.Background(), run)
	if len(sessionIDs) != 4 {
		t.Fatalf("expected four unique session IDs, got %#v", sessionIDs)
	}
	got := map[string]struct{}{}
	for _, id := range sessionIDs {
		got[id] = struct{}{}
	}
	for _, want := range []string{"sess-run", "sess-step", "sess-attempt", "sess-meta"} {
		if _, ok := got[want]; !ok {
			t.Fatalf("expected session %q in resolved set, got %#v", want, sessionIDs)
		}
	}
}

func TestWorkflowRunSessionInterruptExecutorSkipsInactiveAndNotFound(t *testing.T) {
	gateway := &stubWorkflowRunSessionInterruptGateway{
		sessions: map[string]*types.Session{
			"sess-active":   {ID: "sess-active", Status: types.SessionStatusRunning},
			"sess-inactive": {ID: "sess-inactive", Status: types.SessionStatusInactive},
		},
		getErrByID: map[string]error{
			"sess-missing": ErrSessionNotFound,
		},
	}
	executor := &workflowRunSessionInterruptExecutorService{
		sessions: gateway,
		timeout:  0,
	}

	execution, err := executor.InterruptSessions(context.Background(), "gwf-1", []string{"sess-active", "sess-inactive", "sess-missing"})
	if err != nil {
		t.Fatalf("unexpected interrupt error: %v", err)
	}
	if execution.Interrupted != 1 {
		t.Fatalf("expected one interrupted session, got %d", execution.Interrupted)
	}
	if execution.Skipped != 2 {
		t.Fatalf("expected two skipped sessions, got %d", execution.Skipped)
	}
	if !reflect.DeepEqual(gateway.interruptCalls, []string{"sess-active"}) {
		t.Fatalf("expected only active session to be interrupted, got %#v", gateway.interruptCalls)
	}
}

func TestNewWorkflowRunSessionInterruptServiceNilManagerReturnsNil(t *testing.T) {
	service := newWorkflowRunSessionInterruptService(nil, nil, nil, nil)
	if service != nil {
		t.Fatalf("expected nil service when manager is nil")
	}
}

func TestNewWorkflowRunSessionInterruptServiceBuildsWithManager(t *testing.T) {
	service := newWorkflowRunSessionInterruptService(newTestManager(t), nil, nil, nil)
	if service == nil {
		t.Fatalf("expected interrupt service when manager is provided")
	}
}

func TestNewWorkflowRunSessionInterruptServiceUsesProvidedSessionMetaStore(t *testing.T) {
	metaStore := &stubWorkflowRunSessionInterruptMetaStore{}
	service := newWorkflowRunSessionInterruptService(newTestManager(t), &Stores{
		SessionMeta: metaStore,
	}, nil, nil)
	interruptService, ok := service.(*workflowRunSessionInterruptService)
	if !ok || interruptService == nil {
		t.Fatalf("expected concrete workflowRunSessionInterruptService, got %T", service)
	}
	resolver, ok := interruptService.resolver.(*workflowRunSessionTargetResolverService)
	if !ok || resolver == nil {
		t.Fatalf("expected target resolver service, got %T", interruptService.resolver)
	}
	if resolver.sessionMeta != metaStore {
		t.Fatalf("expected resolver to use provided session meta store")
	}
}

func TestWorkflowRunSessionInterruptServiceGuardClauses(t *testing.T) {
	var nilService *workflowRunSessionInterruptService
	if err := nilService.InterruptWorkflowRunSessions(context.Background(), &guidedworkflows.WorkflowRun{ID: "gwf-1"}); err != nil {
		t.Fatalf("expected nil service interrupt to be no-op, got %v", err)
	}

	service := &workflowRunSessionInterruptService{}
	if err := service.InterruptWorkflowRunSessions(context.Background(), &guidedworkflows.WorkflowRun{ID: "gwf-1"}); err != nil {
		t.Fatalf("expected nil resolver/executor interrupt to be no-op, got %v", err)
	}
	service = &workflowRunSessionInterruptService{
		resolver: &stubWorkflowRunSessionTargetResolver{ids: []string{"sess-1"}},
		executor: &stubWorkflowRunSessionInterruptExecutor{},
	}
	if err := service.InterruptWorkflowRunSessions(context.Background(), nil); err != nil {
		t.Fatalf("expected nil run interrupt to be no-op, got %v", err)
	}
	if err := service.InterruptWorkflowRunSessions(context.Background(), &guidedworkflows.WorkflowRun{ID: "   "}); err != nil {
		t.Fatalf("expected blank run id interrupt to be no-op, got %v", err)
	}
}

func TestWorkflowRunSessionInterruptServiceSkipsExecutorWhenNoTargets(t *testing.T) {
	executor := &stubWorkflowRunSessionInterruptExecutor{}
	service := &workflowRunSessionInterruptService{
		resolver: &stubWorkflowRunSessionTargetResolver{ids: nil},
		executor: executor,
	}
	err := service.InterruptWorkflowRunSessions(nil, &guidedworkflows.WorkflowRun{ID: "gwf-1"})
	if err != nil {
		t.Fatalf("expected no-target interrupt to be successful, got %v", err)
	}
	if executor.calls != 0 {
		t.Fatalf("expected executor not to be called when no targets, got %d calls", executor.calls)
	}
}

type stubWorkflowRunSessionInterruptGateway struct {
	sessions         map[string]*types.Session
	getErrByID       map[string]error
	interruptErrByID map[string]error
	getCalls         []string
	interruptCalls   []string
}

func (s *stubWorkflowRunSessionInterruptGateway) Get(_ context.Context, id string) (*types.Session, error) {
	if s == nil {
		return nil, nil
	}
	s.getCalls = append(s.getCalls, id)
	if err, ok := s.getErrByID[id]; ok {
		return nil, err
	}
	session, ok := s.sessions[id]
	if !ok {
		return nil, ErrSessionNotFound
	}
	return session, nil
}

func (s *stubWorkflowRunSessionInterruptGateway) InterruptTurn(_ context.Context, id string) error {
	if s == nil {
		return nil
	}
	s.interruptCalls = append(s.interruptCalls, id)
	if err, ok := s.interruptErrByID[id]; ok {
		return err
	}
	return nil
}

type stubWorkflowRunSessionInterruptMetaStore struct {
	list []*types.SessionMeta
	err  error
}

func (s *stubWorkflowRunSessionInterruptMetaStore) List(context.Context) ([]*types.SessionMeta, error) {
	if s == nil {
		return nil, nil
	}
	if s.err != nil {
		return nil, s.err
	}
	return s.list, nil
}

func (s *stubWorkflowRunSessionInterruptMetaStore) Get(context.Context, string) (*types.SessionMeta, bool, error) {
	return nil, false, nil
}

func (s *stubWorkflowRunSessionInterruptMetaStore) Upsert(context.Context, *types.SessionMeta) (*types.SessionMeta, error) {
	return nil, nil
}

func (s *stubWorkflowRunSessionInterruptMetaStore) Delete(context.Context, string) error {
	return nil
}

type stubWorkflowRunSessionTargetResolver struct {
	ids []string
}

func (s *stubWorkflowRunSessionTargetResolver) ResolveWorkflowRunSessionIDs(context.Context, *guidedworkflows.WorkflowRun) []string {
	if s == nil {
		return nil
	}
	return append([]string(nil), s.ids...)
}

type stubWorkflowRunSessionInterruptExecutor struct {
	calls int
	err   error
}

func (s *stubWorkflowRunSessionInterruptExecutor) InterruptSessions(context.Context, string, []string) (workflowRunSessionInterruptExecution, error) {
	if s == nil {
		return workflowRunSessionInterruptExecution{}, nil
	}
	s.calls++
	return workflowRunSessionInterruptExecution{}, s.err
}
