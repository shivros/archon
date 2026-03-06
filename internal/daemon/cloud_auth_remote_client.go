package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"control/internal/types"
)

type cloudOAuthRemoteClient struct {
	baseURL  string
	clientID string
	client   *http.Client
}

func newCloudOAuthRemoteClient(baseURL, clientID string, timeout time.Duration) *cloudOAuthRemoteClient {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &cloudOAuthRemoteClient{
		baseURL:  strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		clientID: strings.TrimSpace(clientID),
		client:   &http.Client{Timeout: timeout},
	}
}

func (c *cloudOAuthRemoteClient) StartDeviceAuthorization(ctx context.Context, req CloudDeviceAuthorizationRequest) (*types.CloudDeviceAuthorization, error) {
	values := url.Values{}
	values.Set("client_id", firstNonEmptyString(req.ClientID, c.clientID))
	values.Set("installation_id", strings.TrimSpace(req.InstallationID))
	values.Set("device_name", req.DeviceName)
	values.Set("hostname", req.Hostname)
	values.Set("archon_version", req.ArchonVersion)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/oauth/device/code", strings.NewReader(values.Encode()))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var resp types.CloudDeviceAuthorization
	if err := c.do(httpReq, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *cloudOAuthRemoteClient) PollDeviceAuthorization(ctx context.Context, req CloudTokenPollRequest) (*CloudTokenResponse, error) {
	values := url.Values{}
	values.Set("client_id", firstNonEmptyString(req.ClientID, c.clientID))
	values.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	values.Set("device_code", req.DeviceCode)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/oauth/token", strings.NewReader(values.Encode()))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var resp CloudTokenResponse
	if err := c.do(httpReq, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *cloudOAuthRemoteClient) RevokeToken(ctx context.Context, token string) error {
	token = strings.TrimSpace(token)
	if token == "" || strings.TrimSpace(c.baseURL) == "" {
		return nil
	}
	values := url.Values{}
	values.Set("client_id", c.clientID)
	values.Set("token", token)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/oauth/revoke", strings.NewReader(values.Encode()))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return c.do(httpReq, nil)
}

func (c *cloudOAuthRemoteClient) do(req *http.Request, out any) error {
	if c == nil || c.client == nil {
		return errors.New("cloud auth http client is not configured")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload, readErr := io.ReadAll(io.LimitReader(resp.Body, 16*1024))
		if readErr != nil {
			return fmt.Errorf("cloud auth request failed with status %d", resp.StatusCode)
		}
		var oauthPayload struct {
			Error            string `json:"error"`
			ErrorDescription string `json:"error_description"`
		}
		_ = json.Unmarshal(payload, &oauthPayload)
		if code := strings.TrimSpace(oauthPayload.Error); code != "" {
			return &cloudOAuthError{
				StatusCode: resp.StatusCode,
				Code:       code,
				Message:    strings.TrimSpace(oauthPayload.ErrorDescription),
			}
		}
		return fmt.Errorf("cloud auth request failed with status %d", resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
