package daemon

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type errorResponse struct {
	Error string `json:"error"`
}

func TestTokenAuthMiddleware(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mw := TokenAuthMiddleware("secret", handler)

	cases := []struct {
		name       string
		path       string
		authHeader string
		wantStatus int
	}{
		{name: "health-no-auth", path: "/health", wantStatus: http.StatusOK},
		{name: "v1-no-auth", path: "/v1/sessions", wantStatus: http.StatusUnauthorized},
		{name: "v1-wrong-token", path: "/v1/sessions", authHeader: "Bearer nope", wantStatus: http.StatusUnauthorized},
		{name: "v1-correct-token", path: "/v1/sessions", authHeader: "Bearer secret", wantStatus: http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("GET", tc.path, nil)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}

			mw.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if rec.Code == http.StatusUnauthorized {
				var resp errorResponse
				if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
					t.Fatalf("decode json: %v", err)
				}
				if resp.Error == "" {
					t.Fatalf("expected error message")
				}
			}
		})
	}
}
