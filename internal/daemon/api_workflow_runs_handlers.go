package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"

	"control/internal/guidedworkflows"
	"control/internal/logging"
	"control/internal/types"
)

func (a *API) WorkflowRunsEndpoint(w http.ResponseWriter, r *http.Request) {
	service := a.workflowRunService()
	if service == nil {
		writeServiceError(w, unavailableError("guided workflow run service not available", nil))
		return
	}
	switch r.Method {
	case http.MethodGet:
		includeDismissed := parseBoolQueryValue(r.URL.Query().Get("include_dismissed"))
		var (
			runs []*guidedworkflows.WorkflowRun
			err  error
		)
		if includeDismissed {
			runs, err = service.ListRunsIncludingDismissed(r.Context())
		} else {
			runs, err = service.ListRuns(r.Context())
		}
		if err != nil {
			writeServiceError(w, toGuidedWorkflowServiceError(err))
			return
		}
		a.logWorkflowRunsListTelemetry(includeDismissed, runs)
		writeJSON(w, http.StatusOK, map[string]any{"runs": a.presentWorkflowRuns(r.Context(), runs)})
		return
	case http.MethodPost:
		var req CreateWorkflowRunRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		run, err := service.CreateRun(r.Context(), guidedworkflows.CreateRunRequest{
			TemplateID:                strings.TrimSpace(req.TemplateID),
			WorkspaceID:               strings.TrimSpace(req.WorkspaceID),
			WorktreeID:                strings.TrimSpace(req.WorktreeID),
			SessionID:                 strings.TrimSpace(req.SessionID),
			TaskID:                    strings.TrimSpace(req.TaskID),
			UserPrompt:                strings.TrimSpace(req.UserPrompt),
			SelectedProvider:          strings.TrimSpace(req.SelectedProvider),
			SelectedPolicySensitivity: strings.TrimSpace(req.SelectedPolicySensitivity),
			SelectedRuntimeOptions:    types.CloneRuntimeOptions(req.SelectedRuntimeOptions),
			PolicyOverrides:           a.workflowPolicyResolver().ResolvePolicyOverrides(req.PolicyOverrides),
		})
		if err != nil {
			writeServiceError(w, toGuidedWorkflowServiceError(err))
			return
		}
		if a.Logger != nil {
			settings := guidedWorkflowEffectiveDispatchSettings(run.DefaultAccessLevel, a.workflowDispatchDefaults())
			a.Logger.Info("guided_workflow_run_created",
				logging.F("run_id", strings.TrimSpace(run.ID)),
				logging.F("template_id", strings.TrimSpace(run.TemplateID)),
				logging.F("workspace_id", strings.TrimSpace(run.WorkspaceID)),
				logging.F("worktree_id", strings.TrimSpace(run.WorktreeID)),
				logging.F("requested_session_id", strings.TrimSpace(req.SessionID)),
				logging.F("selected_provider", strings.TrimSpace(req.SelectedProvider)),
				logging.F("effective_provider", settings.Provider),
				logging.F("effective_model", settings.Model),
				logging.F("effective_access", settings.Access),
				logging.F("effective_reasoning", settings.Reasoning),
			)
		}
		writeJSON(w, http.StatusCreated, a.presentWorkflowRun(r.Context(), run))
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
	metricsService := a.workflowRunMetricsService()
	if metricsService == nil {
		writeServiceError(w, unavailableError("guided workflow metrics are not available", nil))
		return
	}
	metrics, err := metricsService.GetRunMetrics(r.Context())
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
	resetter := a.workflowRunMetricsResetService()
	if resetter == nil {
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
		a.logWorkflowRunFetchTelemetry(run)
		writeJSON(w, http.StatusOK, a.presentWorkflowRun(r.Context(), run))
		return
	}

	action := strings.TrimSpace(parts[1])
	route, ok := a.workflowRunActionRoutes()[action]
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if r.Method != route.method {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	payload, err := route.handle(r.Context(), r, id, service)
	if err != nil {
		if errors.Is(err, errWorkflowRunInvalidJSONBody) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		writeServiceError(w, toGuidedWorkflowServiceError(err))
		return
	}
	writeJSON(w, http.StatusOK, a.presentWorkflowRunPayload(r.Context(), payload))
}

type workflowRunActionRoute struct {
	method string
	handle func(context.Context, *http.Request, string, GuidedWorkflowRunService) (any, error)
}

var errWorkflowRunInvalidJSONBody = errors.New("invalid json body")

func (a *API) workflowRunActionRoutes() map[string]workflowRunActionRoute {
	return map[string]workflowRunActionRoute{
		"timeline": {
			method: http.MethodGet,
			handle: func(ctx context.Context, _ *http.Request, runID string, service GuidedWorkflowRunService) (any, error) {
				events, err := service.GetRunTimeline(ctx, runID)
				if err != nil {
					return nil, err
				}
				return map[string]any{"timeline": events}, nil
			},
		},
		"start": {
			method: http.MethodPost,
			handle: func(ctx context.Context, _ *http.Request, runID string, service GuidedWorkflowRunService) (any, error) {
				run, err := service.StartRun(ctx, runID)
				if err != nil {
					return nil, err
				}
				a.publishGuidedWorkflowDecisionNotification(run)
				return run, nil
			},
		},
		"pause": {
			method: http.MethodPost,
			handle: func(ctx context.Context, _ *http.Request, runID string, service GuidedWorkflowRunService) (any, error) {
				return service.PauseRun(ctx, runID)
			},
		},
		"stop": {
			method: http.MethodPost,
			handle: func(ctx context.Context, _ *http.Request, runID string, _ GuidedWorkflowRunService) (any, error) {
				coordinator := a.workflowRunStopCoordinator()
				if coordinator == nil {
					return nil, unavailableError("guided workflow stop coordinator not available", nil)
				}
				return coordinator.StopWorkflowRun(ctx, runID)
			},
		},
		"resume": {
			method: http.MethodPost,
			handle: func(ctx context.Context, r *http.Request, runID string, service GuidedWorkflowRunService) (any, error) {
				var req WorkflowRunResumeRequest
				if err := decodeOptionalWorkflowRunActionRequest(r, &req); err != nil {
					return nil, errWorkflowRunInvalidJSONBody
				}
				var (
					run *guidedworkflows.WorkflowRun
					err error
				)
				if req.ResumeFailed {
					run, err = service.ResumeFailedRun(ctx, runID, guidedworkflows.ResumeFailedRunRequest{
						Message: strings.TrimSpace(req.Message),
					})
				} else {
					run, err = service.ResumeRun(ctx, runID)
				}
				if err != nil {
					return nil, err
				}
				a.publishGuidedWorkflowDecisionNotification(run)
				return run, nil
			},
		},
		"rename": {
			method: http.MethodPost,
			handle: func(ctx context.Context, r *http.Request, runID string, service GuidedWorkflowRunService) (any, error) {
				var req WorkflowRunRenameRequest
				if err := decodeOptionalWorkflowRunActionRequest(r, &req); err != nil {
					return nil, errWorkflowRunInvalidJSONBody
				}
				return service.RenameRun(ctx, runID, strings.TrimSpace(req.Name))
			},
		},
		"dismiss": {
			method: http.MethodPost,
			handle: func(ctx context.Context, _ *http.Request, runID string, service GuidedWorkflowRunService) (any, error) {
				run, err := service.DismissRun(ctx, runID)
				if err != nil {
					return nil, err
				}
				a.syncWorkflowSessionVisibility(run, true)
				return run, nil
			},
		},
		"undismiss": {
			method: http.MethodPost,
			handle: func(ctx context.Context, _ *http.Request, runID string, service GuidedWorkflowRunService) (any, error) {
				run, err := service.UndismissRun(ctx, runID)
				if err != nil {
					return nil, err
				}
				a.syncWorkflowSessionVisibility(run, false)
				return run, nil
			},
		},
		"decision": {
			method: http.MethodPost,
			handle: func(ctx context.Context, r *http.Request, runID string, service GuidedWorkflowRunService) (any, error) {
				var req WorkflowRunDecisionRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					return nil, errWorkflowRunInvalidJSONBody
				}
				return service.HandleDecision(ctx, runID, guidedworkflows.DecisionActionRequest{
					Action:     req.Action,
					DecisionID: strings.TrimSpace(req.DecisionID),
					Note:       strings.TrimSpace(req.Note),
				})
			},
		},
	}
}

func decodeOptionalWorkflowRunActionRequest(r *http.Request, dst any) error {
	if r == nil || r.Body == nil || dst == nil {
		return nil
	}
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	return nil
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

func (a *API) syncWorkflowSessionVisibility(run *guidedworkflows.WorkflowRun, dismissed bool) {
	if a == nil || run == nil {
		return
	}
	visibility := a.workflowSessionVisibilityService()
	if visibility == nil {
		syncer := a.workflowRunSessionVisibilitySyncer()
		if syncer == nil {
			return
		}
		visibility = syncer
	}
	visibility.SyncWorkflowRunSessionVisibility(run, dismissed)
}

func (a *API) syncWorkflowRunPrimarySessionVisibility(ctx context.Context, run *guidedworkflows.WorkflowRun, dismissed bool) error {
	syncer := a.workflowRunSessionVisibilitySyncer()
	if syncer == nil {
		return nil
	}
	return syncer.syncWorkflowRunPrimarySessionVisibility(ctx, run, dismissed)
}

func (a *API) syncWorkflowLinkedSessionDismissal(ctx context.Context, run *guidedworkflows.WorkflowRun, dismissed bool) error {
	syncer := a.workflowRunSessionVisibilitySyncer()
	if syncer == nil {
		return nil
	}
	return syncer.syncWorkflowLinkedSessionDismissal(ctx, run, dismissed)
}

func (a *API) presentWorkflowRunPayload(ctx context.Context, payload any) any {
	switch typed := payload.(type) {
	case *guidedworkflows.WorkflowRun:
		return a.presentWorkflowRun(ctx, typed)
	case []*guidedworkflows.WorkflowRun:
		return a.presentWorkflowRuns(ctx, typed)
	default:
		return payload
	}
}

func (a *API) presentWorkflowRuns(ctx context.Context, runs []*guidedworkflows.WorkflowRun) []*guidedworkflows.WorkflowRun {
	if len(runs) == 0 {
		return runs
	}
	resolver := a.workflowRunPromptResolver()
	out := make([]*guidedworkflows.WorkflowRun, 0, len(runs))
	for _, run := range runs {
		out = append(out, presentWorkflowRunWithResolver(ctx, run, resolver))
	}
	return out
}

func (a *API) presentWorkflowRun(ctx context.Context, run *guidedworkflows.WorkflowRun) *guidedworkflows.WorkflowRun {
	return presentWorkflowRunWithResolver(ctx, run, a.workflowRunPromptResolver())
}

func (a *API) workflowRunPromptResolver() workflowRunPromptResolver {
	if a == nil {
		return newWorkflowRunPromptResolver(nil)
	}
	return newWorkflowRunPromptResolver(a.Stores)
}

func presentWorkflowRunWithResolver(ctx context.Context, run *guidedworkflows.WorkflowRun, resolver workflowRunPromptResolver) *guidedworkflows.WorkflowRun {
	if run == nil {
		return nil
	}
	out := cloneWorkflowRunForResponse(run)
	if resolver == nil {
		return out
	}
	out.DisplayUserPrompt = resolver.ResolveDisplayPrompt(ctx, out)
	return out
}

func cloneWorkflowRunForResponse(run *guidedworkflows.WorkflowRun) *guidedworkflows.WorkflowRun {
	if run == nil {
		return nil
	}
	out := *run
	return &out
}

func (a *API) workflowRunSessionVisibilitySyncer() *workflowRunSessionVisibilitySyncService {
	if a == nil {
		return nil
	}
	if syncer, ok := a.workflowSessionVisibilityService().(*workflowRunSessionVisibilitySyncService); ok {
		return syncer
	}
	if a.Stores == nil || a.Stores.SessionMeta == nil {
		return nil
	}
	return &workflowRunSessionVisibilitySyncService{
		sessionMeta: a.Stores.SessionMeta,
		logger:      a.Logger,
		async:       false,
	}
}

func (a *API) logWorkflowRunsListTelemetry(includeDismissed bool, runs []*guidedworkflows.WorkflowRun) {
	if a == nil || a.Logger == nil {
		return
	}
	total := 0
	dismissed := 0
	missingAllContext := 0
	missingWorkspace := 0
	missingWorktree := 0
	missingSession := 0
	statusCounts := map[string]int{}
	sampleIDs := make([]string, 0, 10)
	for _, run := range runs {
		if run == nil {
			continue
		}
		total++
		runID := strings.TrimSpace(run.ID)
		if runID != "" && len(sampleIDs) < 10 {
			sampleIDs = append(sampleIDs, runID)
		}
		if run.DismissedAt != nil {
			dismissed++
		}
		status := strings.TrimSpace(string(run.Status))
		if status == "" {
			status = "unknown"
		}
		statusCounts[status]++
		workspaceID := strings.TrimSpace(run.WorkspaceID)
		worktreeID := strings.TrimSpace(run.WorktreeID)
		sessionID := strings.TrimSpace(run.SessionID)
		if workspaceID == "" {
			missingWorkspace++
		}
		if worktreeID == "" {
			missingWorktree++
		}
		if sessionID == "" {
			missingSession++
		}
		if workspaceID == "" && worktreeID == "" && sessionID == "" {
			missingAllContext++
			a.Logger.Warn("guided_workflow_run_list_missing_context",
				logging.F("run_id", runID),
				logging.F("status", status),
				logging.F("dismissed", run.DismissedAt != nil),
			)
		}
	}
	a.Logger.Info("guided_workflow_runs_listed",
		logging.F("include_dismissed", includeDismissed),
		logging.F("count", total),
		logging.F("dismissed", dismissed),
		logging.F("missing_all_context", missingAllContext),
		logging.F("missing_workspace", missingWorkspace),
		logging.F("missing_worktree", missingWorktree),
		logging.F("missing_session", missingSession),
		logging.F("status_counts", formatWorkflowStatusCounts(statusCounts)),
		logging.F("sample_run_ids", sampleIDs),
	)
}

func (a *API) logWorkflowRunFetchTelemetry(run *guidedworkflows.WorkflowRun) {
	if a == nil || a.Logger == nil || run == nil {
		return
	}
	runID := strings.TrimSpace(run.ID)
	status := strings.TrimSpace(string(run.Status))
	if status == "" {
		status = "unknown"
	}
	workspaceID := strings.TrimSpace(run.WorkspaceID)
	worktreeID := strings.TrimSpace(run.WorktreeID)
	sessionID := strings.TrimSpace(run.SessionID)
	a.Logger.Info("guided_workflow_run_fetched",
		logging.F("run_id", runID),
		logging.F("status", status),
		logging.F("dismissed", run.DismissedAt != nil),
		logging.F("workspace_id", workspaceID),
		logging.F("worktree_id", worktreeID),
		logging.F("session_id", sessionID),
		logging.F("missing_all_context", workspaceID == "" && worktreeID == "" && sessionID == ""),
	)
}

func formatWorkflowStatusCounts(counts map[string]int) string {
	if len(counts) == 0 {
		return ""
	}
	keys := make([]string, 0, len(counts))
	for status := range counts {
		status = strings.TrimSpace(status)
		if status == "" {
			status = "unknown"
		}
		keys = append(keys, status)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, status := range keys {
		parts = append(parts, status+":"+itoa(counts[status]))
	}
	return strings.Join(parts, ",")
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	negative := value < 0
	if negative {
		value = -value
	}
	buf := [32]byte{}
	i := len(buf)
	for value > 0 {
		i--
		buf[i] = byte('0' + (value % 10))
		value /= 10
	}
	if negative {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
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
	case errors.Is(err, guidedworkflows.ErrUnsupportedProvider):
		return invalidError("workflow provider is not dispatchable for guided workflows", err)
	case errors.Is(err, guidedworkflows.ErrInvalidTransition):
		return conflictError("invalid workflow run transition", err)
	case errors.Is(err, guidedworkflows.ErrRunLimitExceeded):
		return conflictError("workflow active run limit exceeded", err)
	case errors.Is(err, guidedworkflows.ErrDisabled):
		return unavailableError("guided workflows are disabled", err)
	case errors.Is(err, guidedworkflows.ErrStepDispatchDeferred):
		return conflictError("guided workflow step prompt deferred: waiting for active turn to finish", err)
	case errors.Is(err, guidedworkflows.ErrStepDispatch):
		if isTurnAlreadyInProgressError(err) {
			return conflictError("guided workflow step prompt blocked: turn already in progress", err)
		}
		return unavailableError("workflow step prompt dispatch unavailable", err)
	default:
		return unavailableError("guided workflow request failed", err)
	}
}
