package daemon

import (
	"net/http"
)

func (a *API) CloudAuthDevice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	service := a.cloudAuthService()
	if service == nil {
		writeServiceError(w, unavailableError("cloud auth is not configured", nil))
		return
	}
	resp, err := service.StartDeviceAuthorization(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *API) CloudAuthStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	service := a.cloudAuthService()
	if service == nil {
		writeJSON(w, http.StatusOK, map[string]any{"linked": false})
		return
	}
	resp, err := service.Status(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *API) CloudAuthPoll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	service := a.cloudAuthService()
	if service == nil {
		writeServiceError(w, unavailableError("cloud auth is not configured", nil))
		return
	}
	resp, err := service.PollDeviceAuthorization(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *API) CloudAuthLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	service := a.cloudAuthService()
	if service == nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	result, err := service.Logout(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
