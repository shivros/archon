package daemon

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"
	"time"

	"control/internal/types"
)

type cloudAuthService struct {
	store          CloudAuthStore
	remote         CloudAuthRemoteClient
	now            func() time.Time
	hostname       func() (string, error)
	version        string
	clientID       string
	baseURL        string
	browserBaseURL string
}

func newCloudAuthService(store CloudAuthStore, remote CloudAuthRemoteClient, baseURL, clientID, version string) *cloudAuthService {
	return &cloudAuthService{
		store:          store,
		remote:         remote,
		now:            time.Now,
		hostname:       os.Hostname,
		version:        strings.TrimSpace(version),
		clientID:       strings.TrimSpace(clientID),
		baseURL:        strings.TrimSpace(baseURL),
		browserBaseURL: strings.TrimSpace(baseURL),
	}
}

func (s *cloudAuthService) StartDeviceAuthorization(ctx context.Context) (*types.CloudDeviceAuthorization, error) {
	if s == nil || s.store == nil || s.remote == nil {
		return nil, unavailableError("cloud auth is not configured", nil)
	}
	if strings.TrimSpace(s.baseURL) == "" {
		return nil, invalidError("cloud base_url is required", nil)
	}
	state, err := s.store.Load(ctx)
	if err != nil {
		return nil, unavailableError("failed to load cloud auth state", err)
	}
	if state == nil {
		state = &CloudAuthState{}
	}
	if strings.TrimSpace(state.InstallationID) == "" {
		installationID, idErr := generateToken()
		if idErr != nil {
			return nil, unavailableError("failed to generate installation id", idErr)
		}
		state.InstallationID = strings.TrimSpace(installationID)
	}
	host := "unknown-host"
	if s.hostname != nil {
		if value, err := s.hostname(); err == nil && strings.TrimSpace(value) != "" {
			host = strings.TrimSpace(value)
		}
	}
	resp, err := s.remote.StartDeviceAuthorization(ctx, CloudDeviceAuthorizationRequest{
		ClientID:       s.clientID,
		InstallationID: state.InstallationID,
		DeviceName:     "Archon on " + host,
		Hostname:       host,
		ArchonVersion:  s.version,
	})
	if err != nil {
		return nil, unavailableError("cloud login start failed", err)
	}
	if resp == nil || strings.TrimSpace(resp.DeviceCode) == "" || strings.TrimSpace(resp.UserCode) == "" || strings.TrimSpace(resp.VerificationURI) == "" {
		return nil, unavailableError("cloud login start returned incomplete payload", nil)
	}
	normalizeCloudDeviceAuthorizationURLs(s.browserBaseURL, resp)
	if resp.Interval <= 0 {
		resp.Interval = 5
	}
	state.Pending = &CloudPendingState{
		DeviceCode:              strings.TrimSpace(resp.DeviceCode),
		UserCode:                strings.TrimSpace(resp.UserCode),
		VerificationURI:         strings.TrimSpace(resp.VerificationURI),
		VerificationURIComplete: strings.TrimSpace(resp.VerificationURIComplete),
		ExpiresAt:               s.now().Add(time.Duration(resp.ExpiresIn) * time.Second).UTC().Format(time.RFC3339),
		Interval:                resp.Interval,
	}
	if err := s.store.Save(ctx, state); err != nil {
		return nil, unavailableError("failed to persist cloud login state", err)
	}
	return resp, nil
}

func (s *cloudAuthService) PollDeviceAuthorization(ctx context.Context) (*types.CloudAuthPollResult, error) {
	if s == nil || s.store == nil || s.remote == nil {
		return nil, unavailableError("cloud auth is not configured", nil)
	}
	state, err := s.store.Load(ctx)
	if err != nil {
		return nil, unavailableError("failed to load cloud auth state", err)
	}
	if state == nil || state.Pending == nil || strings.TrimSpace(state.Pending.DeviceCode) == "" {
		return nil, notFoundError("no cloud login is in progress", nil)
	}
	expiresAt, _ := time.Parse(time.RFC3339, state.Pending.ExpiresAt)
	if !expiresAt.IsZero() && s.now().After(expiresAt) {
		state.Pending = nil
		_ = s.store.Save(ctx, state)
		return nil, conflictError("device authorization expired", nil)
	}
	tokenResp, err := s.remote.PollDeviceAuthorization(ctx, CloudTokenPollRequest{
		ClientID:   s.clientID,
		DeviceCode: state.Pending.DeviceCode,
	})
	if err != nil {
		var oauthErr *cloudOAuthError
		if errors.As(err, &oauthErr) {
			switch oauthErr.Code {
			case "authorization_pending", "slow_down":
				return &types.CloudAuthPollResult{Status: oauthErr.Code}, nil
			case "access_denied":
				state.Pending = nil
				_ = s.store.Save(ctx, state)
				return nil, conflictError("cloud login was denied", oauthErr)
			case "expired_token":
				state.Pending = nil
				_ = s.store.Save(ctx, state)
				return nil, conflictError("device authorization expired", oauthErr)
			}
		}
		return nil, unavailableError("cloud token polling failed", err)
	}
	if tokenResp == nil || strings.TrimSpace(tokenResp.AccessToken) == "" {
		return nil, unavailableError("cloud token response did not include an access token", nil)
	}
	creds := &CloudCredentialsState{
		AccessToken:  strings.TrimSpace(tokenResp.AccessToken),
		RefreshToken: strings.TrimSpace(tokenResp.RefreshToken),
		TokenType:    fallbackTokenType(tokenResp.TokenType),
		Scopes:       splitScope(tokenResp.Scope),
		LinkedAt:     s.now().UTC().Format(time.RFC3339),
		User:         tokenResp.User,
		Installation: tokenResp.Installation,
	}
	if tokenResp.ExpiresIn > 0 {
		creds.Expiry = s.now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second).UTC().Format(time.RFC3339)
	}
	state.Credentials = creds
	state.Pending = nil
	if err := s.store.Save(ctx, state); err != nil {
		return nil, unavailableError("failed to persist cloud credentials", err)
	}
	return &types.CloudAuthPollResult{Status: "approved", Auth: cloudAuthStatusFromState(state)}, nil
}

func (s *cloudAuthService) Status(ctx context.Context) (*types.CloudAuthStatus, error) {
	if s == nil || s.store == nil {
		return &types.CloudAuthStatus{}, nil
	}
	state, err := s.store.Load(ctx)
	if err != nil {
		return nil, unavailableError("failed to load cloud auth state", err)
	}
	return cloudAuthStatusFromState(state), nil
}

func (s *cloudAuthService) Logout(ctx context.Context) (*types.CloudLogoutResult, error) {
	if s == nil || s.store == nil {
		return &types.CloudLogoutResult{Status: "nothing_to_do"}, nil
	}
	state, err := s.store.Load(ctx)
	if err != nil {
		return nil, unavailableError("failed to load cloud auth state", err)
	}
	if state == nil || (state.Credentials == nil && state.Pending == nil) {
		return &types.CloudLogoutResult{Status: "nothing_to_do"}, nil
	}
	result := &types.CloudLogoutResult{}
	if state.Credentials != nil && s.remote != nil && strings.TrimSpace(state.Credentials.AccessToken) != "" {
		if revokeErr := s.remote.RevokeToken(ctx, state.Credentials.AccessToken); revokeErr != nil {
			result.Status = "unlinked_local_only"
			result.Message = "remote revoke failed; cleared local cloud credentials only"
		} else {
			result.RemoteRevoked = true
		}
	}
	state.Credentials = nil
	state.Pending = nil
	if err := s.store.Save(ctx, state); err != nil {
		return nil, unavailableError("failed to clear cloud auth state", err)
	}
	result.LocalCleared = true
	if result.Status == "" {
		result.Status = "revoked_and_unlinked"
		result.Message = "revoked remote token and cleared local cloud credentials"
	}
	return result, nil
}

func cloudAuthStatusFromState(state *CloudAuthState) *types.CloudAuthStatus {
	status := &types.CloudAuthStatus{}
	if state == nil {
		return status
	}
	if creds := state.Credentials; creds != nil && strings.TrimSpace(creds.AccessToken) != "" {
		status.Linked = true
		status.User = creds.User
		status.Installation = creds.Installation
		status.LinkedAt = creds.LinkedAt
		status.TokenType = creds.TokenType
		status.Scopes = append([]string{}, creds.Scopes...)
		status.AccessTokenSet = true
	}
	if pending := state.Pending; pending != nil && strings.TrimSpace(pending.DeviceCode) != "" {
		status.Pending = &types.CloudPendingStatus{
			UserCode:                pending.UserCode,
			VerificationURI:         pending.VerificationURI,
			VerificationURIComplete: pending.VerificationURIComplete,
			ExpiresAt:               pending.ExpiresAt,
			Interval:                pending.Interval,
		}
	}
	return status
}

func splitScope(scope string) []string {
	fields := strings.Fields(strings.TrimSpace(scope))
	if len(fields) == 0 {
		return nil
	}
	return fields
}

func fallbackTokenType(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "Bearer"
	}
	return value
}

func normalizeCloudDeviceAuthorizationURLs(baseURL string, resp *types.CloudDeviceAuthorization) {
	if resp == nil {
		return
	}
	base, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return
	}
	resp.VerificationURI = normalizeCloudVerificationURL(base, resp.VerificationURI)
	resp.VerificationURIComplete = normalizeCloudVerificationURL(base, resp.VerificationURIComplete)
}

func normalizeCloudVerificationURL(base *url.URL, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if base != nil && strings.TrimSpace(base.Host) != "" {
		host := strings.ToLower(parsed.Hostname())
		if host == "" || host == "localhost" || host == "127.0.0.1" || host == "::1" {
			parsed.Scheme = base.Scheme
			parsed.Host = base.Host
		}
	}
	for strings.HasPrefix(parsed.Path, "//") {
		parsed.Path = parsed.Path[1:]
	}
	return parsed.String()
}
