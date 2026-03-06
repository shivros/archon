package types

type CloudDeviceAuthorization struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete,omitempty"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

type CloudLinkedUser struct {
	ID          string `json:"id,omitempty"`
	Email       string `json:"email,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
}

type CloudInstallation struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

type CloudPendingStatus struct {
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete,omitempty"`
	ExpiresAt               string `json:"expires_at"`
	Interval                int    `json:"interval"`
}

type CloudAuthStatus struct {
	Linked         bool                `json:"linked"`
	User           *CloudLinkedUser    `json:"user,omitempty"`
	Installation   *CloudInstallation  `json:"installation,omitempty"`
	LinkedAt       string              `json:"linked_at,omitempty"`
	TokenType      string              `json:"token_type,omitempty"`
	Scopes         []string            `json:"scopes,omitempty"`
	AccessTokenSet bool                `json:"access_token_set,omitempty"`
	Pending        *CloudPendingStatus `json:"pending,omitempty"`
}

type CloudAuthPollResult struct {
	Status string           `json:"status"`
	Auth   *CloudAuthStatus `json:"auth,omitempty"`
}

type CloudLogoutResult struct {
	RemoteRevoked bool   `json:"remote_revoked"`
	LocalCleared  bool   `json:"local_cleared"`
	Status        string `json:"status"`
	Message       string `json:"message,omitempty"`
}
