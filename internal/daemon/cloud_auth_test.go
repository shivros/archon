package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"control/internal/types"
)

type stubCloudAuthStore struct {
	state   *CloudAuthState
	loadErr error
	saveErr error
}

func (s *stubCloudAuthStore) Load(context.Context) (*CloudAuthState, error) {
	if s.loadErr != nil {
		return nil, s.loadErr
	}
	if s.state == nil {
		return &CloudAuthState{}, nil
	}
	data, _ := json.Marshal(s.state)
	var cloned CloudAuthState
	_ = json.Unmarshal(data, &cloned)
	return &cloned, nil
}

func (s *stubCloudAuthStore) Save(_ context.Context, state *CloudAuthState) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	data, _ := json.Marshal(state)
	var cloned CloudAuthState
	_ = json.Unmarshal(data, &cloned)
	s.state = &cloned
	return nil
}

type stubCloudAuthRemote struct {
	startReq   CloudDeviceAuthorizationRequest
	startResp  *types.CloudDeviceAuthorization
	startErr   error
	pollResp   *CloudTokenResponse
	pollErr    error
	revokeErr  error
	revokeCall int
}

func (s *stubCloudAuthRemote) StartDeviceAuthorization(_ context.Context, req CloudDeviceAuthorizationRequest) (*types.CloudDeviceAuthorization, error) {
	s.startReq = req
	return s.startResp, s.startErr
}

func (s *stubCloudAuthRemote) PollDeviceAuthorization(context.Context, CloudTokenPollRequest) (*CloudTokenResponse, error) {
	return s.pollResp, s.pollErr
}

func (s *stubCloudAuthRemote) RevokeToken(context.Context, string) error {
	s.revokeCall++
	return s.revokeErr
}

func TestCloudAuthServiceHappyPath(t *testing.T) {
	store := &stubCloudAuthStore{}
	remote := &stubCloudAuthRemote{
		startResp: &types.CloudDeviceAuthorization{
			DeviceCode:      "dev-1",
			UserCode:        "ABCD-EFGH",
			VerificationURI: "https://archon.example/activate",
			ExpiresIn:       600,
			Interval:        5,
		},
	}
	svc := newCloudAuthService(store, remote, "https://archon.example", "archon-cli", "v-test")
	svc.now = func() time.Time { return time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC) }
	svc.hostname = func() (string, error) { return "test-host", nil }

	started, err := svc.StartDeviceAuthorization(context.Background())
	if err != nil {
		t.Fatalf("StartDeviceAuthorization error: %v", err)
	}
	if started.UserCode != "ABCD-EFGH" {
		t.Fatalf("unexpected start response: %#v", started)
	}
	if store.state == nil || store.state.Pending == nil || store.state.Pending.DeviceCode != "dev-1" {
		t.Fatalf("expected pending state to be persisted, got %#v", store.state)
	}
	if strings.TrimSpace(store.state.InstallationID) == "" || strings.TrimSpace(remote.startReq.InstallationID) == "" {
		t.Fatalf("expected installation id to be persisted and sent, state=%#v req=%#v", store.state, remote.startReq)
	}

	remote.pollResp = &CloudTokenResponse{
		AccessToken:  "access-1",
		RefreshToken: "refresh-1",
		TokenType:    "Bearer",
		Scope:        "profile sync",
		ExpiresIn:    3600,
		User:         &types.CloudLinkedUser{Email: "user@example.com"},
		Installation: &types.CloudInstallation{Name: "Archon Laptop"},
	}
	polled, err := svc.PollDeviceAuthorization(context.Background())
	if err != nil {
		t.Fatalf("PollDeviceAuthorization error: %v", err)
	}
	if polled.Status != "approved" || polled.Auth == nil || !polled.Auth.Linked {
		t.Fatalf("unexpected poll response: %#v", polled)
	}
	if store.state.Pending != nil {
		t.Fatalf("expected pending state to be cleared, got %#v", store.state.Pending)
	}
	if store.state.Credentials == nil || store.state.Credentials.AccessToken != "access-1" {
		t.Fatalf("expected credentials to be persisted, got %#v", store.state.Credentials)
	}
}

func TestCloudAuthServiceLogoutReturnsLocalOnlyResultWhenRevokeFails(t *testing.T) {
	store := &stubCloudAuthStore{
		state: &CloudAuthState{
			Credentials: &CloudCredentialsState{
				AccessToken: "access-1",
			},
		},
	}
	remote := &stubCloudAuthRemote{revokeErr: errors.New("boom")}
	svc := newCloudAuthService(store, remote, "https://archon.example", "archon-cli", "v-test")

	result, err := svc.Logout(context.Background())
	if err != nil {
		t.Fatalf("Logout error: %v", err)
	}
	if result.Status != "unlinked_local_only" || !result.LocalCleared || result.RemoteRevoked {
		t.Fatalf("unexpected logout result: %#v", result)
	}
	if store.state.Credentials != nil {
		t.Fatalf("expected local credentials to be cleared, got %#v", store.state.Credentials)
	}
}

func TestCloudAuthServicePollPending(t *testing.T) {
	store := &stubCloudAuthStore{
		state: &CloudAuthState{
			Pending: &CloudPendingState{
				DeviceCode:      "dev-1",
				UserCode:        "ABCD-EFGH",
				VerificationURI: "https://archon.example/activate",
				ExpiresAt:       time.Now().Add(10 * time.Minute).UTC().Format(time.RFC3339),
				Interval:        5,
			},
		},
	}
	remote := &stubCloudAuthRemote{
		pollErr: &cloudOAuthError{Code: "authorization_pending"},
	}
	svc := newCloudAuthService(store, remote, "https://archon.example", "archon-cli", "v-test")

	polled, err := svc.PollDeviceAuthorization(context.Background())
	if err != nil {
		t.Fatalf("PollDeviceAuthorization error: %v", err)
	}
	if polled.Status != "authorization_pending" {
		t.Fatalf("unexpected poll status: %#v", polled)
	}
}

func TestCloudAuthServiceStartValidationAndFailures(t *testing.T) {
	t.Run("missing base url", func(t *testing.T) {
		svc := newCloudAuthService(&stubCloudAuthStore{}, &stubCloudAuthRemote{}, "", "archon-cli", "v-test")
		if _, err := svc.StartDeviceAuthorization(context.Background()); err == nil {
			t.Fatalf("expected validation error")
		}
	})

	t.Run("load failure", func(t *testing.T) {
		svc := newCloudAuthService(&stubCloudAuthStore{loadErr: errors.New("boom")}, &stubCloudAuthRemote{}, "https://archon.example", "archon-cli", "v-test")
		if _, err := svc.StartDeviceAuthorization(context.Background()); err == nil || !strings.Contains(err.Error(), "failed to load cloud auth state") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("remote start failure", func(t *testing.T) {
		svc := newCloudAuthService(&stubCloudAuthStore{}, &stubCloudAuthRemote{startErr: errors.New("boom")}, "https://archon.example", "archon-cli", "v-test")
		if _, err := svc.StartDeviceAuthorization(context.Background()); err == nil || !strings.Contains(err.Error(), "cloud login start failed") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("incomplete payload", func(t *testing.T) {
		svc := newCloudAuthService(&stubCloudAuthStore{}, &stubCloudAuthRemote{
			startResp: &types.CloudDeviceAuthorization{DeviceCode: "dev-1"},
		}, "https://archon.example", "archon-cli", "v-test")
		if _, err := svc.StartDeviceAuthorization(context.Background()); err == nil || !strings.Contains(err.Error(), "incomplete payload") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("save failure", func(t *testing.T) {
		store := &stubCloudAuthStore{}
		svc := newCloudAuthService(store, &stubCloudAuthRemote{
			startResp: &types.CloudDeviceAuthorization{
				DeviceCode:      "dev-1",
				UserCode:        "ABCD-EFGH",
				VerificationURI: "https://archon.example/activate",
				ExpiresIn:       60,
			},
		}, "https://archon.example", "archon-cli", "v-test")
		store.saveErr = errors.New("save failed")
		if _, err := svc.StartDeviceAuthorization(context.Background()); err == nil || !strings.Contains(err.Error(), "persist cloud login state") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestCloudAuthServicePollFailures(t *testing.T) {
	t.Run("no pending login", func(t *testing.T) {
		svc := newCloudAuthService(&stubCloudAuthStore{}, &stubCloudAuthRemote{}, "https://archon.example", "archon-cli", "v-test")
		if _, err := svc.PollDeviceAuthorization(context.Background()); err == nil || !strings.Contains(err.Error(), "no cloud login is in progress") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("expired pending login", func(t *testing.T) {
		store := &stubCloudAuthStore{state: &CloudAuthState{
			Pending: &CloudPendingState{
				DeviceCode: "dev-1",
				ExpiresAt:  time.Now().Add(-time.Minute).UTC().Format(time.RFC3339),
			},
		}}
		svc := newCloudAuthService(store, &stubCloudAuthRemote{}, "https://archon.example", "archon-cli", "v-test")
		if _, err := svc.PollDeviceAuthorization(context.Background()); err == nil || !strings.Contains(err.Error(), "expired") {
			t.Fatalf("unexpected error: %v", err)
		}
		if store.state.Pending != nil {
			t.Fatalf("expected pending state cleared")
		}
	})

	t.Run("access denied", func(t *testing.T) {
		store := &stubCloudAuthStore{state: &CloudAuthState{Pending: &CloudPendingState{DeviceCode: "dev-1", ExpiresAt: time.Now().Add(time.Minute).UTC().Format(time.RFC3339)}}}
		svc := newCloudAuthService(store, &stubCloudAuthRemote{pollErr: &cloudOAuthError{Code: "access_denied"}}, "https://archon.example", "archon-cli", "v-test")
		if _, err := svc.PollDeviceAuthorization(context.Background()); err == nil || !strings.Contains(err.Error(), "denied") {
			t.Fatalf("unexpected error: %v", err)
		}
		if store.state.Pending != nil {
			t.Fatalf("expected pending state cleared")
		}
	})

	t.Run("expired token", func(t *testing.T) {
		store := &stubCloudAuthStore{state: &CloudAuthState{Pending: &CloudPendingState{DeviceCode: "dev-1", ExpiresAt: time.Now().Add(time.Minute).UTC().Format(time.RFC3339)}}}
		svc := newCloudAuthService(store, &stubCloudAuthRemote{pollErr: &cloudOAuthError{Code: "expired_token"}}, "https://archon.example", "archon-cli", "v-test")
		if _, err := svc.PollDeviceAuthorization(context.Background()); err == nil || !strings.Contains(err.Error(), "expired") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("generic remote error", func(t *testing.T) {
		store := &stubCloudAuthStore{state: &CloudAuthState{Pending: &CloudPendingState{DeviceCode: "dev-1", ExpiresAt: time.Now().Add(time.Minute).UTC().Format(time.RFC3339)}}}
		svc := newCloudAuthService(store, &stubCloudAuthRemote{pollErr: errors.New("boom")}, "https://archon.example", "archon-cli", "v-test")
		if _, err := svc.PollDeviceAuthorization(context.Background()); err == nil || !strings.Contains(err.Error(), "polling failed") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("missing access token", func(t *testing.T) {
		store := &stubCloudAuthStore{state: &CloudAuthState{Pending: &CloudPendingState{DeviceCode: "dev-1", ExpiresAt: time.Now().Add(time.Minute).UTC().Format(time.RFC3339)}}}
		svc := newCloudAuthService(store, &stubCloudAuthRemote{pollResp: &CloudTokenResponse{}}, "https://archon.example", "archon-cli", "v-test")
		if _, err := svc.PollDeviceAuthorization(context.Background()); err == nil || !strings.Contains(err.Error(), "did not include an access token") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("save failure after approval", func(t *testing.T) {
		store := &stubCloudAuthStore{state: &CloudAuthState{Pending: &CloudPendingState{DeviceCode: "dev-1", ExpiresAt: time.Now().Add(time.Minute).UTC().Format(time.RFC3339)}}}
		svc := newCloudAuthService(store, &stubCloudAuthRemote{pollResp: &CloudTokenResponse{AccessToken: "access-1"}}, "https://archon.example", "archon-cli", "v-test")
		store.saveErr = errors.New("save failed")
		if _, err := svc.PollDeviceAuthorization(context.Background()); err == nil || !strings.Contains(err.Error(), "persist cloud credentials") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestCloudAuthServiceStatusAndLogoutEdgeCases(t *testing.T) {
	t.Run("status load failure", func(t *testing.T) {
		svc := newCloudAuthService(&stubCloudAuthStore{loadErr: errors.New("boom")}, &stubCloudAuthRemote{}, "https://archon.example", "archon-cli", "v-test")
		if _, err := svc.Status(context.Background()); err == nil || !strings.Contains(err.Error(), "failed to load cloud auth state") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("status projects pending", func(t *testing.T) {
		svc := newCloudAuthService(&stubCloudAuthStore{state: &CloudAuthState{Pending: &CloudPendingState{DeviceCode: "dev-1", UserCode: "ABCD", VerificationURI: "https://archon.example/activate"}}}, &stubCloudAuthRemote{}, "https://archon.example", "archon-cli", "v-test")
		status, err := svc.Status(context.Background())
		if err != nil || status.Pending == nil || status.Pending.UserCode != "ABCD" {
			t.Fatalf("unexpected status=%#v err=%v", status, err)
		}
	})

	t.Run("logout nothing to do", func(t *testing.T) {
		svc := newCloudAuthService(&stubCloudAuthStore{}, &stubCloudAuthRemote{}, "https://archon.example", "archon-cli", "v-test")
		result, err := svc.Logout(context.Background())
		if err != nil || result.Status != "nothing_to_do" {
			t.Fatalf("unexpected result=%#v err=%v", result, err)
		}
	})

	t.Run("logout revoke success", func(t *testing.T) {
		store := &stubCloudAuthStore{state: &CloudAuthState{Credentials: &CloudCredentialsState{AccessToken: "access-1"}}}
		remote := &stubCloudAuthRemote{}
		svc := newCloudAuthService(store, remote, "https://archon.example", "archon-cli", "v-test")
		result, err := svc.Logout(context.Background())
		if err != nil || result.Status != "revoked_and_unlinked" || !result.RemoteRevoked {
			t.Fatalf("unexpected result=%#v err=%v", result, err)
		}
		if remote.revokeCall != 1 {
			t.Fatalf("expected revoke call")
		}
	})

	t.Run("logout load failure", func(t *testing.T) {
		svc := newCloudAuthService(&stubCloudAuthStore{loadErr: errors.New("boom")}, &stubCloudAuthRemote{}, "https://archon.example", "archon-cli", "v-test")
		if _, err := svc.Logout(context.Background()); err == nil || !strings.Contains(err.Error(), "failed to load cloud auth state") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("logout save failure", func(t *testing.T) {
		store := &stubCloudAuthStore{state: &CloudAuthState{Credentials: &CloudCredentialsState{AccessToken: "access-1"}}}
		svc := newCloudAuthService(store, &stubCloudAuthRemote{}, "https://archon.example", "archon-cli", "v-test")
		store.saveErr = errors.New("save failed")
		if _, err := svc.Logout(context.Background()); err == nil || !strings.Contains(err.Error(), "failed to clear cloud auth state") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestCloudAuthAPIHandlers(t *testing.T) {
	api := &API{
		CloudAuth: &stubCloudAuthServiceForAPI{},
	}
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)
	server := httptest.NewServer(TokenAuthMiddleware("token", mux))
	defer server.Close()

	req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/cloud-auth/device", nil)
	req.Header.Set("Authorization", "Bearer token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("device request error: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected device status: %d", resp.StatusCode)
	}

	statusReq, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/cloud-auth/status", nil)
	statusReq.Header.Set("Authorization", "Bearer token")
	statusResp, err := http.DefaultClient.Do(statusReq)
	if err != nil {
		t.Fatalf("status request error: %v", err)
	}
	defer func() { _ = statusResp.Body.Close() }()
	if statusResp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status code: %d", statusResp.StatusCode)
	}

	pollReq, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/cloud-auth/poll", nil)
	pollReq.Header.Set("Authorization", "Bearer token")
	pollResp, err := http.DefaultClient.Do(pollReq)
	if err != nil {
		t.Fatalf("poll request error: %v", err)
	}
	defer func() { _ = pollResp.Body.Close() }()
	if pollResp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected poll status code: %d", pollResp.StatusCode)
	}

	logoutReq, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/cloud-auth/logout", nil)
	logoutReq.Header.Set("Authorization", "Bearer token")
	logoutResp, err := http.DefaultClient.Do(logoutReq)
	if err != nil {
		t.Fatalf("logout request error: %v", err)
	}
	defer func() { _ = logoutResp.Body.Close() }()
	if logoutResp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected logout status code: %d", logoutResp.StatusCode)
	}
}

func TestCloudAuthAPIHandlersMethodAndNilServiceCases(t *testing.T) {
	t.Run("method not allowed", func(t *testing.T) {
		api := &API{CloudAuth: &stubCloudAuthServiceForAPI{}}
		req := httptest.NewRequest(http.MethodGet, "/v1/cloud-auth/device", nil)
		rec := httptest.NewRecorder()
		api.CloudAuthDevice(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("unexpected status code: %d", rec.Code)
		}
	})

	t.Run("nil service status and logout", func(t *testing.T) {
		api := &API{}
		statusReq := httptest.NewRequest(http.MethodGet, "/v1/cloud-auth/status", nil)
		statusRec := httptest.NewRecorder()
		api.CloudAuthStatusHandler(statusRec, statusReq)
		if statusRec.Code != http.StatusOK || !strings.Contains(statusRec.Body.String(), `"linked":false`) {
			t.Fatalf("unexpected status response: code=%d body=%q", statusRec.Code, statusRec.Body.String())
		}

		logoutReq := httptest.NewRequest(http.MethodPost, "/v1/cloud-auth/logout", nil)
		logoutRec := httptest.NewRecorder()
		api.CloudAuthLogout(logoutRec, logoutReq)
		if logoutRec.Code != http.StatusOK || !strings.Contains(logoutRec.Body.String(), `"ok":true`) {
			t.Fatalf("unexpected logout response: code=%d body=%q", logoutRec.Code, logoutRec.Body.String())
		}
	})
}

type stubCloudAuthServiceForAPI struct{}

func (stubCloudAuthServiceForAPI) StartDeviceAuthorization(context.Context) (*types.CloudDeviceAuthorization, error) {
	return &types.CloudDeviceAuthorization{
		DeviceCode:      "dev-1",
		UserCode:        "ABCD-EFGH",
		VerificationURI: "https://archon.example/activate",
		ExpiresIn:       600,
		Interval:        5,
	}, nil
}

func (stubCloudAuthServiceForAPI) PollDeviceAuthorization(context.Context) (*types.CloudAuthPollResult, error) {
	return &types.CloudAuthPollResult{Status: "authorization_pending"}, nil
}

func (stubCloudAuthServiceForAPI) Status(context.Context) (*types.CloudAuthStatus, error) {
	return &types.CloudAuthStatus{Linked: true}, nil
}

func (stubCloudAuthServiceForAPI) Logout(context.Context) (*types.CloudLogoutResult, error) {
	return &types.CloudLogoutResult{Status: "revoked_and_unlinked", LocalCleared: true, RemoteRevoked: true}, nil
}

func TestCloudOAuthRemoteClientMapsOAuthErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/token" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"authorization_pending","error_description":"waiting"}`))
	}))
	defer server.Close()

	client := newCloudOAuthRemoteClient(server.URL, "archon-cli", time.Second)
	_, err := client.PollDeviceAuthorization(context.Background(), CloudTokenPollRequest{DeviceCode: "dev-1"})
	if err == nil {
		t.Fatalf("expected oauth error")
	}
	var oauthErr *cloudOAuthError
	if !errors.As(err, &oauthErr) || oauthErr.Code != "authorization_pending" || !strings.Contains(oauthErr.Error(), "waiting") {
		t.Fatalf("unexpected error: %#v", err)
	}
}

func TestCloudOAuthRemoteClientSuccessPaths(t *testing.T) {
	var (
		deviceForm url.Values
		tokenForm  url.Values
		revokeForm url.Values
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		form, _ := url.ParseQuery(string(bodyBytes))
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/oauth/device/code":
			deviceForm = form
			_, _ = w.Write([]byte(`{"device_code":"dev-1","user_code":"ABCD-EFGH","verification_uri":"https://archon.example/activate","expires_in":600,"interval":5}`))
		case "/oauth/token":
			tokenForm = form
			_, _ = w.Write([]byte(`{"access_token":"access-1","token_type":"Bearer","expires_in":3600}`))
		case "/oauth/revoke":
			revokeForm = form
			_, _ = w.Write([]byte(`{}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := newCloudOAuthRemoteClient(server.URL, "archon-cli", time.Second)
	if _, err := client.StartDeviceAuthorization(context.Background(), CloudDeviceAuthorizationRequest{
		InstallationID: "install-1",
		DeviceName:     "Archon on host",
		Hostname:       "host",
		ArchonVersion:  "v-test",
	}); err != nil {
		t.Fatalf("StartDeviceAuthorization error: %v", err)
	}
	if deviceForm.Get("client_id") != "archon-cli" || deviceForm.Get("installation_id") != "install-1" {
		t.Fatalf("unexpected device form: %#v", deviceForm)
	}

	if _, err := client.PollDeviceAuthorization(context.Background(), CloudTokenPollRequest{DeviceCode: "dev-1"}); err != nil {
		t.Fatalf("PollDeviceAuthorization error: %v", err)
	}
	if tokenForm.Get("grant_type") != "urn:ietf:params:oauth:grant-type:device_code" || tokenForm.Get("device_code") != "dev-1" {
		t.Fatalf("unexpected token form: %#v", tokenForm)
	}

	if err := client.RevokeToken(context.Background(), "access-1"); err != nil {
		t.Fatalf("RevokeToken error: %v", err)
	}
	if revokeForm.Get("token") != "access-1" {
		t.Fatalf("unexpected revoke form: %#v", revokeForm)
	}
}

func TestCloudOAuthRemoteClientDecodeAndGenericErrors(t *testing.T) {
	t.Run("generic error body", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`oops`))
		}))
		defer server.Close()
		client := newCloudOAuthRemoteClient(server.URL, "archon-cli", time.Second)
		if _, err := client.PollDeviceAuthorization(context.Background(), CloudTokenPollRequest{DeviceCode: "dev-1"}); err == nil || !strings.Contains(err.Error(), "status 502") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("decode failure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{broken`))
		}))
		defer server.Close()
		client := newCloudOAuthRemoteClient(server.URL, "archon-cli", time.Second)
		if _, err := client.PollDeviceAuthorization(context.Background(), CloudTokenPollRequest{DeviceCode: "dev-1"}); err == nil {
			t.Fatalf("expected decode error")
		}
	})
}

func TestFileCloudAuthStoreRoundTripAndErrors(t *testing.T) {
	t.Run("round trip", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "cloud_auth.json")
		store := newFileCloudAuthStore(path)
		state := &CloudAuthState{
			InstallationID: "install-1",
			Credentials:    &CloudCredentialsState{AccessToken: "access-1"},
		}
		if err := store.Save(context.Background(), state); err != nil {
			t.Fatalf("Save error: %v", err)
		}
		loaded, err := store.Load(context.Background())
		if err != nil {
			t.Fatalf("Load error: %v", err)
		}
		if loaded.InstallationID != "install-1" || loaded.Credentials == nil || loaded.Credentials.AccessToken != "access-1" {
			t.Fatalf("unexpected loaded state: %#v", loaded)
		}
	})

	t.Run("missing and blank files", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "cloud_auth.json")
		store := newFileCloudAuthStore(path)
		loaded, err := store.Load(context.Background())
		if err != nil || loaded == nil {
			t.Fatalf("unexpected load result loaded=%#v err=%v", loaded, err)
		}
		if err := os.WriteFile(path, []byte(" \n"), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		loaded, err = store.Load(context.Background())
		if err != nil || loaded == nil {
			t.Fatalf("unexpected blank load result loaded=%#v err=%v", loaded, err)
		}
	})

	t.Run("invalid json and empty path", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "cloud_auth.json")
		if err := os.WriteFile(path, []byte("{broken"), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		store := newFileCloudAuthStore(path)
		if _, err := store.Load(context.Background()); err == nil {
			t.Fatalf("expected invalid json error")
		}
		if err := (*fileCloudAuthStore)(nil).Save(context.Background(), &CloudAuthState{}); err == nil {
			t.Fatalf("expected empty path save error")
		}
	})
}
