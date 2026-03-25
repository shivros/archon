package app

import (
	"strings"

	"control/internal/guidedworkflows"
)

type workflowRunStatusPresentation struct {
	labels map[guidedworkflows.WorkflowRunStatus]string
}

var (
	workflowRunCompactStatusPresentation = workflowRunStatusPresentation{
		labels: map[guidedworkflows.WorkflowRunStatus]string{
			guidedworkflows.WorkflowRunStatusCreated:   "created",
			guidedworkflows.WorkflowRunStatusQueued:    "queued",
			guidedworkflows.WorkflowRunStatusRunning:   "running",
			guidedworkflows.WorkflowRunStatusPaused:    "paused",
			guidedworkflows.WorkflowRunStatusStopped:   "stopped",
			guidedworkflows.WorkflowRunStatusCompleted: "completed",
			guidedworkflows.WorkflowRunStatusFailed:    "failed",
		},
	}
	workflowRunDetailedStatusPresentation = workflowRunStatusPresentation{
		labels: map[guidedworkflows.WorkflowRunStatus]string{
			guidedworkflows.WorkflowRunStatusCreated:   "created",
			guidedworkflows.WorkflowRunStatusQueued:    "queued (waiting for dependencies)",
			guidedworkflows.WorkflowRunStatusRunning:   "running",
			guidedworkflows.WorkflowRunStatusPaused:    "paused (decision needed)",
			guidedworkflows.WorkflowRunStatusStopped:   "stopped",
			guidedworkflows.WorkflowRunStatusCompleted: "completed",
			guidedworkflows.WorkflowRunStatusFailed:    "failed",
		},
	}
)

func workflowRunStatusText(run *guidedworkflows.WorkflowRun) string {
	return workflowRunStatusLabel(run, workflowRunCompactStatusPresentation)
}

func workflowRunDetailedStatusText(run *guidedworkflows.WorkflowRun) string {
	return workflowRunStatusLabel(run, workflowRunDetailedStatusPresentation)
}

func workflowRunStatusLabel(run *guidedworkflows.WorkflowRun, presentation workflowRunStatusPresentation) string {
	if run == nil {
		return ""
	}
	if run.DismissedAt != nil {
		return "dismissed"
	}
	if label, ok := presentation.labels[run.Status]; ok {
		return label
	}
	return strings.TrimSpace(string(run.Status))
}
