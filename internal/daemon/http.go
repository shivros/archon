package daemon

import (
	"encoding/json"
	"net/http"
)

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeServiceError(w http.ResponseWriter, err error) {
	if err == nil {
		return
	}
	status := http.StatusInternalServerError
	message := err.Error()
	if svcErr, ok := err.(*ServiceError); ok {
		switch svcErr.Kind {
		case ServiceErrorInvalid:
			status = http.StatusBadRequest
		case ServiceErrorNotFound:
			status = http.StatusNotFound
		case ServiceErrorConflict:
			status = http.StatusConflict
		case ServiceErrorUnavailable:
			status = http.StatusInternalServerError
		default:
			status = http.StatusInternalServerError
		}
		if svcErr.Message != "" {
			message = svcErr.Message
		}
	}
	writeJSON(w, status, map[string]string{"error": message})
}
