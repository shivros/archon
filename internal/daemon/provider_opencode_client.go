package daemon

import (
	"bytes"
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

type openCodeClient struct {
	baseURL    string
	username   string
	token      string
	timeout    time.Duration
	httpClient *http.Client

	modelResolver  openCodeRuntimeModelResolver
	sessionService *openCodeSessionService
	promptService  *openCodePromptService
	catalogService *openCodeCatalogService
	permissionSvc  *openCodePermissionService
	eventSvc       *openCodeEventService
}

type openCodeClientConfig struct {
	BaseURL  string
	Username string
	Token    string
	Timeout  time.Duration
}

type openCodeRequestError struct {
	Method     string
	Path       string
	StatusCode int
	Message    string
}

func (e *openCodeRequestError) Error() string {
	if e == nil {
		return "opencode request failed"
	}
	msg := strings.TrimSpace(e.Message)
	if msg == "" {
		msg = http.StatusText(e.StatusCode)
	}
	if strings.TrimSpace(msg) == "" {
		msg = "request failed"
	}
	return fmt.Sprintf("opencode request failed (%s %s): %s", e.Method, e.Path, msg)
}

type openCodeModelCatalog struct {
	Models       []string
	DefaultModel string
}

type openCodeParsedProviderCatalog struct {
	ProviderIDs      map[string]struct{}
	ModelToProvider  map[string]string
	NormalizedModels []string
	DefaultModel     string
}

type openCodeModelCatalogIndexProvider interface {
	openCodeModelCatalogIndex(ctx context.Context) (map[string]struct{}, map[string]string, error)
}

type openCodeRuntimeModelResolver interface {
	Resolve(ctx context.Context, runtimeOptions *types.SessionRuntimeOptions) map[string]string
}

type openCodeDefaultRuntimeModelResolver struct {
	catalog openCodeModelCatalogIndexProvider
}

type openCodeSessionMessage struct {
	Info    map[string]any   `json:"info"`
	Parts   []map[string]any `json:"parts"`
	Message map[string]any   `json:"message"`
}

type openCodeAssistantSnapshot struct {
	MessageID string
	Text      string
	CreatedAt time.Time
}

type openCodePermission struct {
	PermissionID string
	SessionID    string
	Status       string
	Kind         string
	Summary      string
	Command      string
	Reason       string
	Metadata     map[string]any
	CreatedAt    time.Time
	Raw          map[string]any
}

func newOpenCodeClient(cfg openCodeClientConfig) (*openCodeClient, error) {
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		return nil, fmt.Errorf("server base_url is required")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base_url: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid base_url: %s", baseURL)
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	username := strings.TrimSpace(cfg.Username)
	if username == "" {
		username = "opencode"
	}
	client := &openCodeClient{
		baseURL:  strings.TrimRight(parsed.String(), "/"),
		username: username,
		token:    strings.TrimSpace(cfg.Token),
		timeout:  timeout,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
	client.catalogService = newOpenCodeCatalogService(client)
	client.sessionService = newOpenCodeSessionService(client)
	client.modelResolver = openCodeDefaultRuntimeModelResolver{catalog: client.catalogService}
	client.promptService = newOpenCodePromptService(client, client.sessionService, client)
	client.permissionSvc = newOpenCodePermissionService(client)
	client.eventSvc = newOpenCodeEventService(client.baseURL, client.username, client.token, client.httpClient.Transport)
	return client, nil
}

func cloneOpenCodeClientWithBaseURL(client *openCodeClient, baseURL string) (*openCodeClient, error) {
	if client == nil {
		return nil, errors.New("client is required")
	}
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" || strings.EqualFold(strings.TrimRight(baseURL, "/"), strings.TrimRight(client.baseURL, "/")) {
		return client, nil
	}
	timeout := client.timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return newOpenCodeClient(openCodeClientConfig{
		BaseURL:  baseURL,
		Username: client.username,
		Token:    client.token,
		Timeout:  timeout,
	})
}

func (c *openCodeClient) CreateSession(ctx context.Context, title, directory string) (string, error) {
	if c == nil || c.sessionService == nil {
		return "", errors.New("session service is required")
	}
	return c.sessionService.CreateSession(ctx, title, directory)
}

func openCodeExtractSessionID(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	if id := strings.TrimSpace(asString(payload["id"])); id != "" {
		return id
	}
	if id := strings.TrimSpace(asString(payload["sessionID"])); id != "" {
		return id
	}
	if session, ok := payload["session"].(map[string]any); ok {
		if id := strings.TrimSpace(asString(session["id"])); id != "" {
			return id
		}
	}
	if info, ok := payload["info"].(map[string]any); ok {
		if id := strings.TrimSpace(asString(info["id"])); id != "" {
			return id
		}
	}
	return ""
}

func (c *openCodeClient) Prompt(ctx context.Context, sessionID, text string, runtimeOptions *types.SessionRuntimeOptions, directory string) (string, error) {
	if c == nil || c.promptService == nil {
		return "", errors.New("prompt service is required")
	}
	return c.promptService.Prompt(ctx, sessionID, text, runtimeOptions, directory)
}

func (c *openCodeClient) resolveRuntimeModel(ctx context.Context, runtimeOptions *types.SessionRuntimeOptions) map[string]string {
	if c == nil || c.modelResolver == nil {
		return nil
	}
	return c.modelResolver.Resolve(ctx, runtimeOptions)
}

func (r openCodeDefaultRuntimeModelResolver) Resolve(ctx context.Context, runtimeOptions *types.SessionRuntimeOptions) map[string]string {
	if runtimeOptions == nil {
		return nil
	}
	raw := strings.TrimSpace(runtimeOptions.Model)
	if raw == "" {
		return nil
	}
	if !strings.Contains(raw, "/") {
		return map[string]string{"modelID": raw}
	}
	parts := strings.SplitN(raw, "/", 2)
	providerID := strings.TrimSpace(parts[0])
	modelID := strings.TrimSpace(parts[1])
	if providerID == "" || modelID == "" {
		return map[string]string{"modelID": raw}
	}

	// Preferred format: provider-prefixed model id ("provider/model-id...").
	if strings.Contains(modelID, "/") {
		return map[string]string{
			"providerID": providerID,
			"modelID":    modelID,
		}
	}

	// Legacy format from older Archon builds ("vendor/model-id") can be ambiguous.
	// Resolve provider id from current server catalog when possible.
	if r.catalog != nil {
		providers, modelToProvider, err := r.catalog.openCodeModelCatalogIndex(ctx)
		if err == nil {
			if _, ok := providers[providerID]; ok {
				return map[string]string{
					"providerID": providerID,
					"modelID":    modelID,
				}
			}
			if resolvedProvider := strings.TrimSpace(modelToProvider[raw]); resolvedProvider != "" {
				return map[string]string{
					"providerID": resolvedProvider,
					"modelID":    raw,
				}
			}
		}
	}

	// Safe fallback: keep the full model id and let the server route it.
	// This avoids sending invalid provider/model pairs when provider ids differ
	// from model-id prefixes (for example, openrouter/google/...).
	return map[string]string{"modelID": raw}
}

func (c *openCodeClient) openCodeModelCatalogIndex(ctx context.Context) (map[string]struct{}, map[string]string, error) {
	if c == nil || c.catalogService == nil {
		return nil, nil, errors.New("model catalog service is required")
	}
	return c.catalogService.openCodeModelCatalogIndex(ctx)
}

func cloneStringSet(src map[string]struct{}) map[string]struct{} {
	if len(src) == 0 {
		return map[string]struct{}{}
	}
	out := make(map[string]struct{}, len(src))
	for key, value := range src {
		out[key] = value
	}
	return out
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(src))
	for key, value := range src {
		out[key] = value
	}
	return out
}

func (c *openCodeClient) AbortSession(ctx context.Context, sessionID, directory string) error {
	if c == nil || c.sessionService == nil {
		return errors.New("session service is required")
	}
	return c.sessionService.AbortSession(ctx, sessionID, directory)
}

func (c *openCodeClient) ListSessionMessages(ctx context.Context, sessionID, directory string, limit int) ([]openCodeSessionMessage, error) {
	if c == nil || c.sessionService == nil {
		return nil, errors.New("session service is required")
	}
	return c.sessionService.listSessionMessages(ctx, sessionID, directory, limit)
}

func (c *openCodeClient) ListModels(ctx context.Context) (*openCodeModelCatalog, error) {
	if c == nil || c.catalogService == nil {
		return nil, errors.New("model catalog service is required")
	}
	return c.catalogService.ListModels(ctx)
}

func (c *openCodeClient) ListPermissions(ctx context.Context, sessionID, directory string) ([]openCodePermission, error) {
	if c == nil || c.permissionSvc == nil {
		return nil, errors.New("permission service is required")
	}
	return c.permissionSvc.ListPermissions(ctx, sessionID, directory)
}

func (c *openCodeClient) ReplyPermission(ctx context.Context, sessionID, permissionID, decision string, responses []string, directory string) error {
	if c == nil || c.permissionSvc == nil {
		return errors.New("permission service is required")
	}
	return c.permissionSvc.ReplyPermission(ctx, sessionID, permissionID, decision, responses, directory)
}

func (c *openCodeClient) SubscribeSessionEvents(ctx context.Context, sessionID, directory string) (<-chan types.CodexEvent, func(), error) {
	if c == nil || c.eventSvc == nil {
		return nil, nil, errors.New("event service is required")
	}
	return c.eventSvc.SubscribeSessionEvents(ctx, sessionID, directory)
}

func (c *openCodeClient) doJSON(ctx context.Context, method, path string, body any, out any) error {
	path = "/" + strings.TrimLeft(strings.TrimSpace(path), "/")
	endpoint := c.baseURL + path

	var payload io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, payload)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.SetBasicAuth(c.username, c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		msg := strings.TrimSpace(string(raw))
		if msg == "" {
			msg = resp.Status
		}
		return &openCodeRequestError{
			Method:     method,
			Path:       path,
			StatusCode: resp.StatusCode,
			Message:    msg,
		}
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func openCodeShouldFallbackLegacy(err error) bool {
	reqErr, ok := err.(*openCodeRequestError)
	if !ok || reqErr == nil {
		return false
	}
	switch reqErr.StatusCode {
	case http.StatusNotFound, http.StatusMethodNotAllowed:
		return true
	default:
		return false
	}
}

func appendOpenCodeDirectoryQuery(path, directory string) string {
	directory = strings.TrimSpace(directory)
	if directory == "" {
		return path
	}
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	return path + separator + "directory=" + url.QueryEscape(directory)
}
