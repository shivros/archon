package daemon

import "net/http"

func (a *API) WorkflowTemplatesEndpoint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	service := a.workflowTemplateService()
	if service == nil {
		writeServiceError(w, unavailableError("guided workflow template service not available", nil))
		return
	}
	templates, err := service.ListTemplates(r.Context())
	if err != nil {
		writeServiceError(w, toGuidedWorkflowServiceError(err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"templates": templates})
}
