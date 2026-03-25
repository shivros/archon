package app

import (
	"strings"

	"control/internal/guidedworkflows"
)

type workflowRunSemanticState int

const (
	workflowRunStateUnknown workflowRunSemanticState = iota
	workflowRunStateDismissed
	workflowRunStateCreated
	workflowRunStateQueued
	workflowRunStateRunning
	workflowRunStatePaused
	workflowRunStateStopped
	workflowRunStateCompleted
	workflowRunStateFailed
)

type workflowRunStatusPresentation struct {
	labels map[workflowRunSemanticState]string
}

var (
	workflowRunSidebarStatusPresentation = workflowRunStatusPresentation{
		labels: map[workflowRunSemanticState]string{
			workflowRunStateDismissed: "dismissed",
			workflowRunStateCreated:   "new",
			workflowRunStateQueued:    "waiting",
			workflowRunStateRunning:   "",
			workflowRunStatePaused:    "paused",
			workflowRunStateStopped:   "stopped",
			workflowRunStateCompleted: "",
			workflowRunStateFailed:    "failed",
		},
	}
	workflowRunDetailedStatusPresentation = workflowRunStatusPresentation{
		labels: map[workflowRunSemanticState]string{
			workflowRunStateDismissed: "dismissed",
			workflowRunStateCreated:   "created",
			workflowRunStateQueued:    "queued (waiting for dependencies)",
			workflowRunStateRunning:   "running",
			workflowRunStatePaused:    "paused (decision needed)",
			workflowRunStateStopped:   "stopped",
			workflowRunStateCompleted: "completed",
			workflowRunStateFailed:    "failed",
		},
	}
)

func workflowRunSidebarStatusText(run *guidedworkflows.WorkflowRun) string {
	return workflowRunStatusLabel(run, workflowRunSidebarStatusPresentation)
}

func workflowRunDetailedStatusText(run *guidedworkflows.WorkflowRun) string {
	return workflowRunStatusLabel(run, workflowRunDetailedStatusPresentation)
}

func workflowRunStatusLabel(run *guidedworkflows.WorkflowRun, presentation workflowRunStatusPresentation) string {
	if run == nil {
		return ""
	}
	state := workflowRunSemanticStateForRun(run)
	if label, ok := presentation.labels[state]; ok {
		return label
	}
	return strings.TrimSpace(string(run.Status))
}

func workflowRunSemanticStateForRun(run *guidedworkflows.WorkflowRun) workflowRunSemanticState {
	if run == nil {
		return workflowRunStateUnknown
	}
	if run.DismissedAt != nil {
		return workflowRunStateDismissed
	}
	switch run.Status {
	case guidedworkflows.WorkflowRunStatusCreated:
		return workflowRunStateCreated
	case guidedworkflows.WorkflowRunStatusQueued:
		return workflowRunStateQueued
	case guidedworkflows.WorkflowRunStatusRunning:
		return workflowRunStateRunning
	case guidedworkflows.WorkflowRunStatusPaused:
		return workflowRunStatePaused
	case guidedworkflows.WorkflowRunStatusStopped:
		return workflowRunStateStopped
	case guidedworkflows.WorkflowRunStatusCompleted:
		return workflowRunStateCompleted
	case guidedworkflows.WorkflowRunStatusFailed:
		return workflowRunStateFailed
	default:
		return workflowRunStateUnknown
	}
}
