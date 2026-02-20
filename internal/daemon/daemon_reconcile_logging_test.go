package daemon

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"control/internal/logging"
)

func TestLogGuidedWorkflowRunReconciliationOutcome(t *testing.T) {
	t.Run("logs warning on reconcile error", func(t *testing.T) {
		var out bytes.Buffer
		logger := logging.New(&out, logging.Info)
		logGuidedWorkflowRunReconciliationOutcome(logger, guidedWorkflowRunSnapshotReconciliationResult{}, errors.New("boom"))
		logs := out.String()
		if !strings.Contains(logs, "msg=guided_workflow_runs_reconcile_failed") {
			t.Fatalf("expected reconcile warning log, got %q", logs)
		}
	})

	t.Run("logs info when runs created", func(t *testing.T) {
		var out bytes.Buffer
		logger := logging.New(&out, logging.Info)
		logGuidedWorkflowRunReconciliationOutcome(logger, guidedWorkflowRunSnapshotReconciliationResult{
			CreatedSnapshots: 2,
			SkippedExisting:  3,
		}, nil)
		logs := out.String()
		if !strings.Contains(logs, "msg=guided_workflow_runs_reconciled_from_session_meta") {
			t.Fatalf("expected reconcile info log, got %q", logs)
		}
		if !strings.Contains(logs, "created_runs=2") {
			t.Fatalf("expected created_runs field in log output, got %q", logs)
		}
	})

	t.Run("logs info when writes fail", func(t *testing.T) {
		var out bytes.Buffer
		logger := logging.New(&out, logging.Info)
		logGuidedWorkflowRunReconciliationOutcome(logger, guidedWorkflowRunSnapshotReconciliationResult{
			FailedWrites: 1,
		}, nil)
		logs := out.String()
		if !strings.Contains(logs, "msg=guided_workflow_runs_reconciled_from_session_meta") {
			t.Fatalf("expected reconcile info log for failed writes, got %q", logs)
		}
		if !strings.Contains(logs, "failed_writes=1") {
			t.Fatalf("expected failed_writes field in log output, got %q", logs)
		}
	})

	t.Run("stays silent on zero result and nil error", func(t *testing.T) {
		var out bytes.Buffer
		logger := logging.New(&out, logging.Info)
		logGuidedWorkflowRunReconciliationOutcome(logger, guidedWorkflowRunSnapshotReconciliationResult{}, nil)
		if logs := strings.TrimSpace(out.String()); logs != "" {
			t.Fatalf("expected no logs for empty result, got %q", logs)
		}
	})

	t.Run("no panic on nil logger", func(t *testing.T) {
		logGuidedWorkflowRunReconciliationOutcome(nil, guidedWorkflowRunSnapshotReconciliationResult{
			CreatedSnapshots: 1,
		}, nil)
	})
}
