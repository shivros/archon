package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"control/internal/guidedworkflows"
)

type guidedWorkflowRunMetricsProvider interface {
	GetRunMetrics(ctx context.Context) (guidedworkflows.RunMetricsSnapshot, error)
}

type guidedWorkflowRunMetricsResetter interface {
	ResetRunMetrics(ctx context.Context) (guidedworkflows.RunMetricsSnapshot, error)
}

func (a *API) WorkflowRunsEndpoint(w http.ResponseWriter, r *http.Request) {
	service := a.workflowRunService()
	if service == nil {
		writeServiceError(w, unavailableError("guided workflow run service not available", nil))
		return
	}
	switch r.Method {
	case http.MethodGet:
		runs, err := service.ListRuns(r.Context())
		if err != nil {
			writeServiceError(w, toGuidedWorkflowServiceError(err))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"runs": runs})
		return
	case http.MethodPost:
		var req CreateWorkflowRunRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		run, err := service.CreateRun(r.Context(), guidedworkflows.CreateRunRequest{
			TemplateID:      strings.TrimSpace(req.TemplateID),
			WorkspaceID:     strings.TrimSpace(req.WorkspaceID),
			WorktreeID:      strings.TrimSpace(req.WorktreeID),
			SessionID:       strings.TrimSpace(req.SessionID),
			TaskID:          strings.TrimSpace(req.TaskID),
			UserPrompt:      strings.TrimSpace(req.UserPrompt),
			PolicyOverrides: req.PolicyOverrides,
		})
		if err != nil {
			writeServiceError(w, toGuidedWorkflowServiceError(err))
			return
		}
		writeJSON(w, http.StatusCreated, run)
		return
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
}

func (a *API) WorkflowRunMetricsEndpoint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	service := a.workflowRunService()
	if service == nil {
		writeServiceError(w, unavailableError("guided workflow run service not available", nil))
		return
	}
	metricsProvider, ok := any(service).(guidedWorkflowRunMetricsProvider)
	if !ok {
		writeServiceError(w, unavailableError("guided workflow metrics are not available", nil))
		return
	}
	metrics, err := metricsProvider.GetRunMetrics(r.Context())
	if err != nil {
		writeServiceError(w, toGuidedWorkflowServiceError(err))
		return
	}
	writeJSON(w, http.StatusOK, metrics)
}

func (a *API) WorkflowRunMetricsResetEndpoint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	service := a.workflowRunService()
	if service == nil {
		writeServiceError(w, unavailableError("guided workflow run service not available", nil))
		return
	}
	resetter, ok := any(service).(guidedWorkflowRunMetricsResetter)
	if !ok {
		writeServiceError(w, unavailableError("guided workflow metrics reset is not available", nil))
		return
	}
	metrics, err := resetter.ResetRunMetrics(r.Context())
	if err != nil {
		writeServiceError(w, toGuidedWorkflowServiceError(err))
		return
	}
	writeJSON(w, http.StatusOK, metrics)
}

func (a *API) WorkflowRunByID(w http.ResponseWriter, r *http.Request) {
	service := a.workflowRunService()
	if service == nil {
		writeServiceError(w, unavailableError("guided workflow run service not available", nil))
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/v1/workflow-runs/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	id := strings.TrimSpace(parts[0])

	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		run, err := service.GetRun(r.Context(), id)
		if err != nil {
			writeServiceError(w, toGuidedWorkflowServiceError(err))
			return
		}
		writeJSON(w, http.StatusOK, run)
		return
	}

	action := strings.TrimSpace(parts[1])
	switch action {
	case "timeline":
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		events, err := service.GetRunTimeline(r.Context(), id)
		if err != nil {
			writeServiceError(w, toGuidedWorkflowServiceError(err))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"timeline": events})
		return
	case "start":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		run, err := service.StartRun(r.Context(), id)
		if err != nil {
			writeServiceError(w, toGuidedWorkflowServiceError(err))
			return
		}
		a.publishGuidedWorkflowDecisionNotification(run)
		writeJSON(w, http.StatusOK, run)
		return
	case "pause":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		run, err := service.PauseRun(r.Context(), id)
		if err != nil {
			writeServiceError(w, toGuidedWorkflowServiceError(err))
			return
		}
		writeJSON(w, http.StatusOK, run)
		return
	case "resume":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		run, err := service.ResumeRun(r.Context(), id)
		if err != nil {
			writeServiceError(w, toGuidedWorkflowServiceError(err))
			return
		}
		a.publishGuidedWorkflowDecisionNotification(run)
		writeJSON(w, http.StatusOK, run)
		return
	case "decision":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		var req WorkflowRunDecisionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		run, err := service.HandleDecision(r.Context(), id, guidedworkflows.DecisionActionRequest{
			Action:     req.Action,
			DecisionID: strings.TrimSpace(req.DecisionID),
			Note:       strings.TrimSpace(req.Note),
		})
		if err != nil {
			writeServiceError(w, toGuidedWorkflowServiceError(err))
			return
		}
		writeJSON(w, http.StatusOK, run)
		return
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
}

func (a *API) publishGuidedWorkflowDecisionNotification(run *guidedworkflows.WorkflowRun) {
	if a == nil || a.Notifier == nil {
		return
	}
	event, ok := guidedWorkflowDecisionNotificationEvent(nil, run)
	if !ok {
		return
	}
	a.Notifier.Publish(event)
}

func toGuidedWorkflowServiceError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, guidedworkflows.ErrRunNotFound):
		return notFoundError("workflow run not found", err)
	case errors.Is(err, guidedworkflows.ErrTemplateNotFound):
		return invalidError("workflow template not found", err)
	case errors.Is(err, guidedworkflows.ErrMissingContext):
		return invalidError("workspace_id or worktree_id is required", err)
	case errors.Is(err, guidedworkflows.ErrInvalidTransition):
		return conflictError("invalid workflow run transition", err)
	case errors.Is(err, guidedworkflows.ErrRunLimitExceeded):
		return conflictError("workflow active run limit exceeded", err)
	case errors.Is(err, guidedworkflows.ErrDisabled):
		return unavailableError("guided workflows are disabled", err)
	default:
		return unavailableError("guided workflow request failed", err)
	}
}
