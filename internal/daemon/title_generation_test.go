package daemon

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"control/internal/guidedworkflows"
	"control/internal/logging"
	"control/internal/store"
	"control/internal/types"
)

type stubTitleGenerator struct {
	title string
	err   error
}

func (s stubTitleGenerator) GenerateTitle(context.Context, string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	return s.title, nil
}

type captureSessionTitleUpdater struct {
	calls chan SessionTitleGenerationRequest
}

func (u captureSessionTitleUpdater) TryUpdateGeneratedSessionTitle(
	_ context.Context,
	sessionID,
	expectedTitle,
	generatedTitle string,
) (bool, error) {
	if u.calls != nil {
		u.calls <- SessionTitleGenerationRequest{SessionID: sessionID, ExpectedTitle: expectedTitle, Prompt: generatedTitle}
	}
	return true, nil
}

type captureWorkflowTitleUpdater struct {
	calls chan WorkflowTitleGenerationRequest
}

func (u captureWorkflowTitleUpdater) TryUpdateGeneratedWorkflowTitle(
	_ context.Context,
	runID,
	expectedTitle,
	generatedTitle string,
) (bool, error) {
	if u.calls != nil {
		u.calls <- WorkflowTitleGenerationRequest{RunID: runID, ExpectedTitle: expectedTitle, Prompt: generatedTitle}
	}
	return true, nil
}

type errorSessionTitleUpdater struct {
	err error
}

func (u errorSessionTitleUpdater) TryUpdateGeneratedSessionTitle(
	_ context.Context,
	_,
	_,
	_ string,
) (bool, error) {
	if u.err != nil {
		return false, u.err
	}
	return false, nil
}

type stubWorkflowRunTitleService struct {
	run       *guidedworkflows.WorkflowRun
	getErr    error
	renameErr error
}

func (s stubWorkflowRunTitleService) GetRun(context.Context, string) (*guidedworkflows.WorkflowRun, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	return s.run, nil
}

func (s stubWorkflowRunTitleService) RenameRun(context.Context, string, string) (*guidedworkflows.WorkflowRun, error) {
	if s.renameErr != nil {
		return nil, s.renameErr
	}
	return s.run, nil
}

type stubCoreConfigReader struct {
	timeout int
}

func (s stubCoreConfigReader) TitleGenerationTimeoutSeconds() int {
	return s.timeout
}

func TestAsyncTitleGenerationServiceProcessesSessionJobs(t *testing.T) {
	updates := make(chan SessionTitleGenerationRequest, 1)
	svc := newAsyncTitleGenerationService(
		stubTitleGenerator{title: "Generated Session Title"},
		captureSessionTitleUpdater{calls: updates},
		nil,
		nil,
		titleGenerationWorkerOptions{Timeout: 2 * time.Second, Buffer: 4},
	)
	defer svc.Close()

	svc.EnqueueSessionTitle(SessionTitleGenerationRequest{
		SessionID:     "sess-1",
		Prompt:        "fix workflow issue",
		ExpectedTitle: "fix workflow issue",
	})

	select {
	case got := <-updates:
		if got.SessionID != "sess-1" {
			t.Fatalf("expected session id sess-1, got %q", got.SessionID)
		}
		if got.ExpectedTitle != "fix workflow issue" {
			t.Fatalf("unexpected expected title: %q", got.ExpectedTitle)
		}
		if got.Prompt != "Generated Session Title" {
			t.Fatalf("unexpected generated title: %q", got.Prompt)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for title generation update")
	}
}

func TestAsyncTitleGenerationServiceProcessesWorkflowJobs(t *testing.T) {
	updates := make(chan WorkflowTitleGenerationRequest, 1)
	svc := newAsyncTitleGenerationService(
		stubTitleGenerator{title: "Generated Workflow Title"},
		nil,
		captureWorkflowTitleUpdater{calls: updates},
		nil,
		titleGenerationWorkerOptions{Timeout: 2 * time.Second, Buffer: 4},
	)
	defer svc.Close()

	svc.EnqueueWorkflowTitle(WorkflowTitleGenerationRequest{
		RunID:         "run-1",
		Prompt:        "do release automation",
		ExpectedTitle: "SOLID Phase Delivery",
	})

	select {
	case got := <-updates:
		if got.RunID != "run-1" {
			t.Fatalf("expected run id run-1, got %q", got.RunID)
		}
		if got.ExpectedTitle != "SOLID Phase Delivery" {
			t.Fatalf("unexpected expected title: %q", got.ExpectedTitle)
		}
		if got.Prompt != "Generated Workflow Title" {
			t.Fatalf("unexpected generated title: %q", got.Prompt)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for workflow title generation update")
	}
}

func TestAsyncTitleGenerationServiceEnqueueDropsWhenQueueFull(t *testing.T) {
	svc := &asyncTitleGenerationService{
		jobs:   make(chan titleGenerationJob, 1),
		closed: make(chan struct{}),
		logger: logging.Nop(),
	}
	svc.jobs <- titleGenerationJob{kind: titleGenerationJobSession, targetID: "existing", prompt: "first"}
	svc.enqueue(titleGenerationJob{kind: titleGenerationJobSession, targetID: "new", prompt: "second"})
	if len(svc.jobs) != 1 {
		t.Fatalf("expected queue to remain full at len=1, got %d", len(svc.jobs))
	}
	job := <-svc.jobs
	if job.targetID != "existing" {
		t.Fatalf("expected existing queued job to remain, got %+v", job)
	}
}

func TestAsyncTitleGenerationServiceEnqueueSkipsWhenClosed(t *testing.T) {
	svc := &asyncTitleGenerationService{
		jobs:   make(chan titleGenerationJob, 1),
		closed: make(chan struct{}),
		logger: logging.Nop(),
	}
	close(svc.closed)
	svc.enqueue(titleGenerationJob{kind: titleGenerationJobSession, targetID: "s1", prompt: "prompt"})
	if len(svc.jobs) != 0 {
		t.Fatalf("expected no enqueue on closed service, got len=%d", len(svc.jobs))
	}
}

func TestAsyncTitleGenerationServiceProcessSkipsOnGeneratorError(t *testing.T) {
	updates := make(chan SessionTitleGenerationRequest, 1)
	svc := &asyncTitleGenerationService{
		generator:      stubTitleGenerator{err: errors.New("boom")},
		sessionUpdater: captureSessionTitleUpdater{calls: updates},
		logger:         logging.Nop(),
		timeout:        time.Second,
	}
	svc.process(titleGenerationJob{kind: titleGenerationJobSession, targetID: "s1", prompt: "prompt"})
	select {
	case got := <-updates:
		t.Fatalf("did not expect session update when generator errors, got %+v", got)
	default:
	}
}

func TestAsyncTitleGenerationServiceProcessSkipsOnEmptyGeneratedTitle(t *testing.T) {
	updates := make(chan SessionTitleGenerationRequest, 1)
	svc := &asyncTitleGenerationService{
		generator:      stubTitleGenerator{title: "   "},
		sessionUpdater: captureSessionTitleUpdater{calls: updates},
		logger:         logging.Nop(),
		timeout:        time.Second,
	}
	svc.process(titleGenerationJob{kind: titleGenerationJobSession, targetID: "s1", prompt: "prompt"})
	select {
	case got := <-updates:
		t.Fatalf("did not expect session update for empty generated title, got %+v", got)
	default:
	}
}

func TestAsyncTitleGenerationServiceProcessHandlesUpdaterError(t *testing.T) {
	svc := &asyncTitleGenerationService{
		generator:      stubTitleGenerator{title: "Generated"},
		sessionUpdater: errorSessionTitleUpdater{err: errors.New("cannot apply")},
		logger:         logging.Nop(),
		timeout:        time.Second,
	}
	svc.process(titleGenerationJob{kind: titleGenerationJobSession, targetID: "s1", prompt: "prompt"})
}

func TestDefaultGeneratedSessionTitleUpdaterSkipsLockedTitle(t *testing.T) {
	stores := &Stores{
		Sessions:    storeSessionsIndex(t),
		SessionMeta: storeSessionMetaStore(t),
	}
	now := time.Now().UTC()
	_, err := stores.Sessions.UpsertRecord(context.Background(), &types.SessionRecord{
		Session: &types.Session{ID: "sess-lock", Title: "Fallback", Provider: "custom", CreatedAt: now},
		Source:  "internal",
	})
	if err != nil {
		t.Fatalf("upsert session: %v", err)
	}
	_, err = stores.SessionMeta.Upsert(context.Background(), &types.SessionMeta{SessionID: "sess-lock", Title: "Fallback", TitleLocked: true, LastActiveAt: &now})
	if err != nil {
		t.Fatalf("upsert meta: %v", err)
	}

	updater := newDefaultGeneratedSessionTitleUpdater(nil, stores, nil)
	updated, err := updater.TryUpdateGeneratedSessionTitle(context.Background(), "sess-lock", "Fallback", "Generated")
	if err != nil {
		t.Fatalf("TryUpdateGeneratedSessionTitle: %v", err)
	}
	if updated {
		t.Fatalf("expected locked title to skip updates")
	}
}

func TestDefaultGeneratedWorkflowTitleUpdaterSkipsWhenTitleChanged(t *testing.T) {
	runs := guidedworkflows.NewRunService(guidedworkflows.Config{Enabled: true})
	run, err := runs.CreateRun(context.Background(), guidedworkflows.CreateRunRequest{WorkspaceID: "ws-1", WorktreeID: "wt-1"})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if _, err := runs.RenameRun(context.Background(), run.ID, "Manual Rename"); err != nil {
		t.Fatalf("RenameRun: %v", err)
	}
	updater := newDefaultGeneratedWorkflowTitleUpdater(runs)
	updated, err := updater.TryUpdateGeneratedWorkflowTitle(context.Background(), run.ID, "SOLID Phase Delivery", "AI Title")
	if err != nil {
		t.Fatalf("TryUpdateGeneratedWorkflowTitle: %v", err)
	}
	if updated {
		t.Fatalf("expected updater to skip when title no longer matches expected")
	}
}

func TestDefaultGeneratedSessionTitleUpdaterAppliesWhenExpectedMatches(t *testing.T) {
	stores := &Stores{
		Sessions:    storeSessionsIndex(t),
		SessionMeta: storeSessionMetaStore(t),
	}
	manager := newTestManager(t)
	manager.SetSessionStore(stores.Sessions)
	manager.SetMetaStore(stores.SessionMeta)
	now := time.Now().UTC()
	_, err := stores.Sessions.UpsertRecord(context.Background(), &types.SessionRecord{
		Session: &types.Session{ID: "sess-apply", Title: "Fallback", Provider: "custom", CreatedAt: now},
		Source:  "internal",
	})
	if err != nil {
		t.Fatalf("upsert session: %v", err)
	}
	_, err = stores.SessionMeta.Upsert(context.Background(), &types.SessionMeta{SessionID: "sess-apply", Title: "Fallback", LastActiveAt: &now})
	if err != nil {
		t.Fatalf("upsert meta: %v", err)
	}

	updater := newDefaultGeneratedSessionTitleUpdater(manager, stores, nil)
	updated, err := updater.TryUpdateGeneratedSessionTitle(context.Background(), "sess-apply", "Fallback", "Generated")
	if err != nil {
		t.Fatalf("TryUpdateGeneratedSessionTitle: %v", err)
	}
	if !updated {
		t.Fatalf("expected session title to update")
	}
	meta, ok, err := stores.SessionMeta.Get(context.Background(), "sess-apply")
	if err != nil {
		t.Fatalf("meta get: %v", err)
	}
	if !ok || meta == nil {
		t.Fatalf("expected meta record")
	}
	if meta.TitleLocked {
		t.Fatalf("expected generated title update to keep title unlocked")
	}
	if strings.TrimSpace(meta.Title) != "Generated" {
		t.Fatalf("expected generated title in metadata, got %q", meta.Title)
	}
}

func TestDefaultGeneratedSessionTitleUpdaterSkipsExpectedMismatch(t *testing.T) {
	stores := &Stores{
		Sessions:    storeSessionsIndex(t),
		SessionMeta: storeSessionMetaStore(t),
	}
	now := time.Now().UTC()
	_, err := stores.Sessions.UpsertRecord(context.Background(), &types.SessionRecord{
		Session: &types.Session{ID: "sess-mismatch", Title: "Fallback", Provider: "custom", CreatedAt: now},
		Source:  "internal",
	})
	if err != nil {
		t.Fatalf("upsert session: %v", err)
	}
	updater := newDefaultGeneratedSessionTitleUpdater(nil, stores, nil)
	updated, err := updater.TryUpdateGeneratedSessionTitle(context.Background(), "sess-mismatch", "Different", "Generated")
	if err != nil {
		t.Fatalf("TryUpdateGeneratedSessionTitle: %v", err)
	}
	if updated {
		t.Fatalf("expected mismatch to skip update")
	}
}

func TestDefaultGeneratedSessionTitleUpdaterSkipsMissingSession(t *testing.T) {
	stores := &Stores{
		Sessions:    storeSessionsIndex(t),
		SessionMeta: storeSessionMetaStore(t),
	}
	updater := newDefaultGeneratedSessionTitleUpdater(nil, stores, nil)
	updated, err := updater.TryUpdateGeneratedSessionTitle(context.Background(), "sess-missing", "Fallback", "Generated")
	if err != nil {
		t.Fatalf("TryUpdateGeneratedSessionTitle: %v", err)
	}
	if updated {
		t.Fatalf("expected missing session to skip update")
	}
}

func storeSessionMetaStore(t *testing.T) *store.FileSessionMetaStore {
	t.Helper()
	return store.NewFileSessionMetaStore(t.TempDir() + "/sessions_meta.json")
}

func TestCaptureQueueThreadSafety(t *testing.T) {
	q := &captureTitleGenerationQueue{}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			q.EnqueueSessionTitle(SessionTitleGenerationRequest{SessionID: "s", Prompt: "p"})
			q.EnqueueWorkflowTitle(WorkflowTitleGenerationRequest{RunID: "r", Prompt: "p"})
		}(i)
	}
	wg.Wait()
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.sessionRequests) != 20 || len(q.workflowRequests) != 20 {
		t.Fatalf("unexpected queue counts: sessions=%d workflows=%d", len(q.sessionRequests), len(q.workflowRequests))
	}
}

func TestDefaultGeneratedWorkflowTitleUpdaterSkipsWhenSameTitle(t *testing.T) {
	updater := newDefaultGeneratedWorkflowTitleUpdater(stubWorkflowRunTitleService{
		run: &guidedworkflows.WorkflowRun{ID: "run-1", TemplateName: "Same"},
	})
	updated, err := updater.TryUpdateGeneratedWorkflowTitle(context.Background(), "run-1", "Same", "Same")
	if err != nil {
		t.Fatalf("TryUpdateGeneratedWorkflowTitle: %v", err)
	}
	if updated {
		t.Fatalf("expected no-op when generated title equals current title")
	}
}

func TestDefaultGeneratedWorkflowTitleUpdaterReturnsRenameError(t *testing.T) {
	updater := newDefaultGeneratedWorkflowTitleUpdater(stubWorkflowRunTitleService{
		run:       &guidedworkflows.WorkflowRun{ID: "run-1", TemplateName: "Old"},
		renameErr: errors.New("rename failed"),
	})
	_, err := updater.TryUpdateGeneratedWorkflowTitle(context.Background(), "run-1", "Old", "New")
	if err == nil {
		t.Fatalf("expected rename error")
	}
}

func TestGeneratedSessionTitleUpdaterPublishesMetadataEvent(t *testing.T) {
	stores := &Stores{
		Sessions:    storeSessionsIndex(t),
		SessionMeta: storeSessionMetaStore(t),
	}
	manager, err := NewSessionManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewSessionManager: %v", err)
	}
	manager.SetSessionStore(stores.Sessions)
	manager.SetMetaStore(stores.SessionMeta)
	hub := newMetadataEventHub(nil)
	manager.SetMetadataEventPublisher(hub)
	ch, cancel, err := hub.Subscribe("")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cancel()

	now := time.Now().UTC()
	if _, err := stores.Sessions.UpsertRecord(context.Background(), &types.SessionRecord{
		Session: &types.Session{
			ID:        "sess-ai",
			Provider:  "custom",
			Title:     "Old",
			Status:    types.SessionStatusInactive,
			CreatedAt: now,
		},
		Source: sessionSourceInternal,
	}); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	updater := newDefaultGeneratedSessionTitleUpdater(manager, stores, nil)
	updated, err := updater.TryUpdateGeneratedSessionTitle(context.Background(), "sess-ai", "Old", "New Title")
	if err != nil {
		t.Fatalf("TryUpdateGeneratedSessionTitle: %v", err)
	}
	if !updated {
		t.Fatalf("expected session title update")
	}
	select {
	case event := <-ch:
		if event.Type != types.MetadataEventTypeSessionUpdated {
			t.Fatalf("unexpected event type: %q", event.Type)
		}
		if event.Session == nil || event.Session.ID != "sess-ai" || event.Session.Title != "New Title" {
			t.Fatalf("unexpected session payload: %#v", event.Session)
		}
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for metadata event")
	}
}

func TestGeneratedWorkflowTitleUpdaterPublishesMetadataEvent(t *testing.T) {
	hub := newMetadataEventHub(nil)
	runService := guidedworkflows.NewRunService(
		guidedworkflows.Config{Enabled: true},
		guidedworkflows.WithMetadataEventPublisher(newGuidedWorkflowMetadataEventAdapter(hub)),
	)
	t.Cleanup(runService.Close)
	run, err := runService.CreateRun(context.Background(), guidedworkflows.CreateRunRequest{
		WorkspaceID: "ws-1",
		UserPrompt:  "ship it",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	ch, cancel, err := hub.Subscribe("")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cancel()

	updater := newDefaultGeneratedWorkflowTitleUpdater(runService)
	updated, err := updater.TryUpdateGeneratedWorkflowTitle(context.Background(), run.ID, run.TemplateName, "AI Workflow")
	if err != nil {
		t.Fatalf("TryUpdateGeneratedWorkflowTitle: %v", err)
	}
	if !updated {
		t.Fatalf("expected workflow title update")
	}
	select {
	case event := <-ch:
		if event.Type != types.MetadataEventTypeWorkflowRunUpdated {
			t.Fatalf("unexpected event type: %q", event.Type)
		}
		if event.Workflow == nil || event.Workflow.ID != run.ID || event.Workflow.Title != "AI Workflow" {
			t.Fatalf("unexpected workflow payload: %#v", event.Workflow)
		}
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for metadata event")
	}
}

func TestTitleGenerationWorkerOptionsFromCoreConfig(t *testing.T) {
	gotDefault := titleGenerationWorkerOptionsFromCoreConfig(nil)
	if gotDefault.Timeout != 10*time.Second || gotDefault.Buffer != 128 {
		t.Fatalf("unexpected default worker options: %+v", gotDefault)
	}
	gotCustom := titleGenerationWorkerOptionsFromCoreConfig(stubCoreConfigReader{timeout: 25})
	if gotCustom.Timeout != 25*time.Second || gotCustom.Buffer != 128 {
		t.Fatalf("unexpected custom worker options: %+v", gotCustom)
	}
}
