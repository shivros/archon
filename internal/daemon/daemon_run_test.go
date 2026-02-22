package daemon

import (
	"context"
	"sync/atomic"
	"testing"

	"control/internal/config"
	"control/internal/guidedworkflows"
	"control/internal/logging"
)

type trackedGuidedWorkflowRunService struct {
	guidedworkflows.RunService
	closeCalls atomic.Int32
}

func (s *trackedGuidedWorkflowRunService) Close() {
	if s == nil {
		return
	}
	s.closeCalls.Add(1)
	if closer, ok := any(s.RunService).(interface{ Close() }); ok && closer != nil {
		closer.Close()
	}
}

func TestDaemonRunClosesWorkflowRunServiceOnShutdown(t *testing.T) {
	previousFactory := newGuidedWorkflowRunServiceFn
	t.Cleanup(func() {
		newGuidedWorkflowRunServiceFn = previousFactory
	})

	trackedService := &trackedGuidedWorkflowRunService{
		RunService: guidedworkflows.NewRunService(guidedworkflows.Config{Enabled: true}),
	}
	newGuidedWorkflowRunServiceFn = func(
		config.CoreConfig,
		*Stores,
		*SessionManager,
		*CodexLiveManager,
		logging.Logger,
	) guidedworkflows.RunService {
		return trackedService
	}

	daemon := New("127.0.0.1:0", "token", "test-version", nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := daemon.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := trackedService.closeCalls.Load(); got != 1 {
		t.Fatalf("expected workflow run service close exactly once, got %d", got)
	}
}
