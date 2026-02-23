package daemon

import (
	"context"
	"strings"

	"control/internal/guidedworkflows"
	"control/internal/logging"
)

type workflowRunStopCoordinator struct {
	runs      GuidedWorkflowRunService
	interrupt WorkflowRunSessionInterruptService
	logger    logging.Logger
}

func newWorkflowRunStopCoordinator(
	runs GuidedWorkflowRunService,
	interrupt WorkflowRunSessionInterruptService,
	logger logging.Logger,
) WorkflowRunStopCoordinator {
	if runs == nil {
		return nil
	}
	return &workflowRunStopCoordinator{
		runs:      runs,
		interrupt: interrupt,
		logger:    logger,
	}
}

func (c *workflowRunStopCoordinator) StopWorkflowRun(
	ctx context.Context,
	runID string,
) (*guidedworkflows.WorkflowRun, error) {
	if c == nil || c.runs == nil {
		return nil, unavailableError("guided workflow run service not available", nil)
	}
	run, err := c.runs.StopRun(ctx, strings.TrimSpace(runID))
	if err != nil {
		return nil, err
	}
	if c.interrupt == nil || run == nil {
		return run, nil
	}
	if err := c.interrupt.InterruptWorkflowRunSessions(ctx, run); err != nil && c.logger != nil {
		c.logger.Warn("guided_workflow_run_session_interrupt_error",
			logging.F("run_id", strings.TrimSpace(run.ID)),
			logging.F("run_session_id", strings.TrimSpace(run.SessionID)),
			logging.F("error", err),
		)
	}
	return run, nil
}
