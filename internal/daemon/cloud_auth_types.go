package daemon

import (
	"context"
	"strings"

	"control/internal/types"
)

type CloudAuthService interface {
	StartDeviceAuthorization(ctx context.Context) (*types.CloudDeviceAuthorization, error)
	PollDeviceAuthorization(ctx context.Context) (*types.CloudAuthPollResult, error)
	Status(ctx context.Context) (*types.CloudAuthStatus, error)
	Logout(ctx context.Context) (*types.CloudLogoutResult, error)
}

type CloudAuthRemoteClient interface {
	StartDeviceAuthorization(ctx context.Context, req CloudDeviceAuthorizationRequest) (*types.CloudDeviceAuthorization, error)
	PollDeviceAuthorization(ctx context.Context, req CloudTokenPollRequest) (*CloudTokenResponse, error)
	RevokeToken(ctx context.Context, token string) error
}

type CloudAuthStore interface {
	Load(ctx context.Context) (*CloudAuthState, error)
	Save(ctx context.Context, state *CloudAuthState) error
}

type CloudDeviceAuthorizationRequest struct {
	ClientID       string
	InstallationID string
	DeviceName     string
	Hostname       string
	ArchonVersion  string
}

type CloudTokenPollRequest struct {
	ClientID   string
	DeviceCode string
}

type CloudTokenResponse struct {
	AccessToken  string                   `json:"access_token"`
	RefreshToken string                   `json:"refresh_token,omitempty"`
	TokenType    string                   `json:"token_type,omitempty"`
	Scope        string                   `json:"scope,omitempty"`
	ExpiresIn    int                      `json:"expires_in,omitempty"`
	User         *types.CloudLinkedUser   `json:"user,omitempty"`
	Installation *types.CloudInstallation `json:"installation,omitempty"`
}

type CloudAuthState struct {
	InstallationID string                 `json:"installation_id,omitempty"`
	Credentials    *CloudCredentialsState `json:"credentials,omitempty"`
	Pending        *CloudPendingState     `json:"pending,omitempty"`
}

type CloudCredentialsState struct {
	AccessToken  string                   `json:"access_token"`
	RefreshToken string                   `json:"refresh_token,omitempty"`
	TokenType    string                   `json:"token_type,omitempty"`
	Scopes       []string                 `json:"scopes,omitempty"`
	Expiry       string                   `json:"expiry,omitempty"`
	LinkedAt     string                   `json:"linked_at"`
	User         *types.CloudLinkedUser   `json:"user,omitempty"`
	Installation *types.CloudInstallation `json:"installation,omitempty"`
}

type CloudPendingState struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete,omitempty"`
	ExpiresAt               string `json:"expires_at"`
	Interval                int    `json:"interval"`
}

type cloudOAuthError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *cloudOAuthError) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.Message) != "" {
		return e.Message
	}
	return e.Code
}
