package daemon

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"control/internal/types"
)

type openCodeClient struct {
	baseURL    string
	username   string
	token      string
	timeout    time.Duration
	httpClient *http.Client

	modelResolver     openCodeRuntimeModelResolver
	catalogMu         sync.RWMutex
	catalogFetchedAt  time.Time
	catalogProviders  map[string]struct{}
	catalogModelToPID map[string]string
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
	client.modelResolver = openCodeDefaultRuntimeModelResolver{catalog: client}
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
	payload := map[string]any{}
	if strings.TrimSpace(title) != "" {
		payload["title"] = strings.TrimSpace(title)
	}
	var result map[string]any
	path := appendOpenCodeDirectoryQuery("/session", directory)
	if err := c.doJSON(ctx, http.MethodPost, path, payload, &result); err != nil {
		if errors.Is(err, io.EOF) {
			return c.lookupCreatedSessionID(ctx, title, directory)
		}
		return "", err
	}
	if sessionID := openCodeExtractSessionID(result); sessionID != "" {
		return sessionID, nil
	}
	sessionID, lookupErr := c.lookupCreatedSessionID(ctx, title, directory)
	if lookupErr == nil && strings.TrimSpace(sessionID) != "" {
		return sessionID, nil
	}
	if lookupErr != nil {
		return "", lookupErr
	}
	return "", fmt.Errorf("session id missing from server response")
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

func (c *openCodeClient) lookupCreatedSessionID(ctx context.Context, title, directory string) (string, error) {
	query := url.Values{}
	if dir := strings.TrimSpace(directory); dir != "" {
		query.Set("directory", dir)
	}
	if q := strings.TrimSpace(title); q != "" {
		query.Set("search", q)
	}
	query.Set("limit", "20")
	path := "/session"
	if encoded := strings.TrimSpace(query.Encode()); encoded != "" {
		path += "?" + encoded
	}
	var sessions []map[string]any
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &sessions); err != nil {
		return "", err
	}
	if len(sessions) == 0 {
		return "", fmt.Errorf("session id missing from server response")
	}
	title = strings.TrimSpace(title)
	if title != "" {
		for _, session := range sessions {
			if strings.EqualFold(strings.TrimSpace(asString(session["title"])), title) {
				if id := openCodeExtractSessionID(session); id != "" {
					return id, nil
				}
			}
		}
	}
	for _, session := range sessions {
		if id := openCodeExtractSessionID(session); id != "" {
			return id, nil
		}
	}
	return "", fmt.Errorf("session id missing from server response")
}

func (c *openCodeClient) Prompt(ctx context.Context, sessionID, text string, runtimeOptions *types.SessionRuntimeOptions, directory string) (string, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "", fmt.Errorf("session id is required")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", fmt.Errorf("text is required")
	}
	body := map[string]any{
		"parts": []map[string]any{
			{
				"type": "text",
				"text": text,
			},
		},
	}
	if model := c.resolveRuntimeModel(ctx, runtimeOptions); len(model) > 0 {
		body["model"] = model
	}

	baseline := c.latestAssistantSnapshot(ctx, sessionID, directory)

	var result openCodeSessionMessage
	paths := []string{
		appendOpenCodeDirectoryQuery(fmt.Sprintf("/session/%s/message", url.PathEscape(sessionID)), directory),
		appendOpenCodeDirectoryQuery(fmt.Sprintf("/session/%s/prompt", url.PathEscape(sessionID)), directory),
	}
	var lastErr error
	for idx, path := range paths {
		err := c.doJSON(ctx, http.MethodPost, path, body, &result)
		if err == nil {
			reply := extractOpenCodeSessionMessageText(result)
			if reply != "" {
				return reply, nil
			}
			// Some servers acknowledge the request before the assistant payload is available.
			if recovered := c.waitForAssistantReply(ctx, sessionID, directory, baseline); recovered != "" {
				return recovered, nil
			}
			return "", nil
		}
		if errors.Is(err, io.EOF) {
			// Some server builds can acknowledge prompt/message with 2xx and an empty body.
			// Attempt to recover the resulting assistant message from session history.
			if recovered := c.waitForAssistantReply(ctx, sessionID, directory, baseline); recovered != "" {
				return recovered, nil
			}
			return "", nil
		}
		lastErr = err
		if idx == 0 && openCodeShouldFallbackLegacy(err) {
			continue
		}
		return "", err
	}
	return "", lastErr
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
	if c == nil {
		return nil, nil, errors.New("client is required")
	}
	c.catalogMu.RLock()
	if c.catalogProviders != nil && c.catalogModelToPID != nil && time.Since(c.catalogFetchedAt) < 30*time.Second {
		providers := cloneStringSet(c.catalogProviders)
		modelToProvider := cloneStringMap(c.catalogModelToPID)
		c.catalogMu.RUnlock()
		return providers, modelToProvider, nil
	}
	c.catalogMu.RUnlock()

	var payload struct {
		Providers []map[string]any `json:"providers"`
		Default   map[string]any   `json:"default"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/config/providers", nil, &payload); err != nil {
		return nil, nil, err
	}

	parsed := openCodeParseProviderCatalog(payload.Providers, payload.Default)

	c.catalogMu.Lock()
	c.catalogProviders = cloneStringSet(parsed.ProviderIDs)
	c.catalogModelToPID = cloneStringMap(parsed.ModelToProvider)
	c.catalogFetchedAt = time.Now()
	c.catalogMu.Unlock()

	return parsed.ProviderIDs, parsed.ModelToProvider, nil
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
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return fmt.Errorf("session id is required")
	}
	path := appendOpenCodeDirectoryQuery(fmt.Sprintf("/session/%s/abort", url.PathEscape(sessionID)), directory)
	return c.doJSON(ctx, http.MethodPost, path, map[string]any{}, nil)
}

func (c *openCodeClient) latestAssistantSnapshot(ctx context.Context, sessionID, directory string) openCodeAssistantSnapshot {
	messages, err := c.listSessionMessages(ctx, sessionID, directory, 8)
	if err != nil {
		return openCodeAssistantSnapshot{}
	}
	return openCodeLatestAssistantSnapshot(messages)
}

func (c *openCodeClient) waitForAssistantReply(ctx context.Context, sessionID, directory string, baseline openCodeAssistantSnapshot) string {
	deadline := time.Now().Add(12 * time.Second)
	delay := 250 * time.Millisecond
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return ""
		}
		messages, err := c.listSessionMessages(ctx, sessionID, directory, 20)
		if err == nil {
			current := openCodeLatestAssistantSnapshot(messages)
			if openCodeAssistantChanged(current, baseline) {
				return current.Text
			}
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ""
		case <-timer.C:
		}
		if delay < time.Second {
			delay *= 2
		}
	}
	return ""
}

func (c *openCodeClient) listSessionMessages(ctx context.Context, sessionID, directory string, limit int) ([]openCodeSessionMessage, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("session id is required")
	}
	path := fmt.Sprintf("/session/%s/message", url.PathEscape(sessionID))
	query := url.Values{}
	if limit > 0 {
		query.Set("limit", strconv.Itoa(limit))
	}
	if encoded := strings.TrimSpace(query.Encode()); encoded != "" {
		path += "?" + encoded
	}
	pathWithDirectory := appendOpenCodeDirectoryQuery(path, directory)

	var payload any
	if err := c.doJSON(ctx, http.MethodGet, pathWithDirectory, nil, &payload); err != nil {
		if strings.TrimSpace(directory) == "" || !openCodeShouldFallbackLegacy(err) {
			return nil, err
		}
		payload = nil
		if retryErr := c.doJSON(ctx, http.MethodGet, path, nil, &payload); retryErr != nil {
			return nil, retryErr
		}
	}
	return normalizeOpenCodeSessionMessages(payload), nil
}

func (c *openCodeClient) ListModels(ctx context.Context) (*openCodeModelCatalog, error) {
	var payload struct {
		Providers []map[string]any `json:"providers"`
		Default   map[string]any   `json:"default"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/config/providers", nil, &payload); err != nil {
		return nil, err
	}

	parsed := openCodeParseProviderCatalog(payload.Providers, payload.Default)
	return &openCodeModelCatalog{
		Models:       parsed.NormalizedModels,
		DefaultModel: parsed.DefaultModel,
	}, nil
}

func (c *openCodeClient) ListPermissions(ctx context.Context, sessionID, directory string) ([]openCodePermission, error) {
	path := "/permission"
	query := url.Values{}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID != "" {
		query.Set("sessionID", sessionID)
	}
	if dir := strings.TrimSpace(directory); dir != "" {
		query.Set("directory", dir)
	}
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}
	var payload any
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &payload); err != nil {
		if sessionID != "" && openCodeShouldFallbackLegacy(err) {
			if retryErr := c.doJSON(ctx, http.MethodGet, "/permission", nil, &payload); retryErr == nil {
				// continue below with unfiltered list
			} else if openCodeShouldFallbackLegacy(retryErr) {
				// Newer server versions expose approvals only over events.
				return []openCodePermission{}, nil
			} else {
				return nil, retryErr
			}
		} else if openCodeShouldFallbackLegacy(err) {
			return []openCodePermission{}, nil
		} else {
			return nil, err
		}
	}
	entries := normalizeOpenCodePermissionList(payload)
	out := make([]openCodePermission, 0, len(entries))
	for _, item := range entries {
		parsed, ok := parseOpenCodePermission(item)
		if !ok {
			continue
		}
		if sessionID != "" && parsed.SessionID != "" && parsed.SessionID != sessionID {
			continue
		}
		out = append(out, parsed)
	}
	return out, nil
}

func (c *openCodeClient) ReplyPermission(ctx context.Context, sessionID, permissionID, decision, directory string) error {
	sessionID = strings.TrimSpace(sessionID)
	permissionID = strings.TrimSpace(permissionID)
	if permissionID == "" {
		return fmt.Errorf("permission id is required")
	}
	legacyDecision := normalizeApprovalDecision(decision)
	modernDecision := normalizeOpenCodePermissionResponse(legacyDecision)
	if sessionID != "" {
		path := appendOpenCodeDirectoryQuery(
			fmt.Sprintf("/session/%s/permissions/%s", url.PathEscape(sessionID), url.PathEscape(permissionID)),
			directory,
		)
		body := map[string]any{
			"response": modernDecision,
		}
		if err := c.doJSON(ctx, http.MethodPost, path, body, nil); err == nil {
			return nil
		} else if !openCodeShouldFallbackLegacy(err) {
			return err
		}
	}
	legacyPath := appendOpenCodeDirectoryQuery("/permission/"+url.PathEscape(permissionID)+"/reply", directory)
	legacyBody := map[string]any{
		"decision": legacyDecision,
		"response": legacyDecision,
		"value":    legacyDecision,
	}
	return c.doJSON(ctx, http.MethodPost, legacyPath, legacyBody, nil)
}

func (c *openCodeClient) SubscribeSessionEvents(ctx context.Context, sessionID, directory string) (<-chan types.CodexEvent, func(), error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, nil, fmt.Errorf("session id is required")
	}
	streamCtx, streamCancel := context.WithCancel(ctx)
	endpoint := c.baseURL + "/event"
	if dir := strings.TrimSpace(directory); dir != "" {
		query := url.Values{}
		query.Set("directory", dir)
		endpoint += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(streamCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		streamCancel()
		return nil, nil, err
	}
	req.Header.Set("Accept", "text/event-stream")
	if c.token != "" {
		req.SetBasicAuth(c.username, c.token)
	}

	streamClient := &http.Client{
		Transport: c.httpClient.Transport,
	}
	resp, err := streamClient.Do(req)
	if err != nil {
		streamCancel()
		return nil, nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		streamCancel()
		return nil, nil, &openCodeRequestError{
			Method:     http.MethodGet,
			Path:       "/event",
			StatusCode: resp.StatusCode,
			Message:    strings.TrimSpace(string(raw)),
		}
	}

	out := make(chan types.CodexEvent, 256)
	usedPermissionIDs := map[int]string{}
	go func() {
		defer close(out)
		defer streamCancel()
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		dataLines := make([]string, 0, 8)
		emit := func(payload string) bool {
			payload = strings.TrimSpace(payload)
			if payload == "" {
				return true
			}
			events := mapOpenCodeEventToCodex(payload, sessionID, usedPermissionIDs)
			if len(events) == 0 {
				return true
			}
			for _, event := range events {
				select {
				case <-streamCtx.Done():
					return false
				case out <- event:
				}
			}
			return true
		}

		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "data:") {
				dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
				continue
			}
			if line != "" {
				continue
			}
			if len(dataLines) == 0 {
				continue
			}
			if !emit(strings.Join(dataLines, "\n")) {
				return
			}
			dataLines = dataLines[:0]
		}
		if len(dataLines) > 0 {
			_ = emit(strings.Join(dataLines, "\n"))
		}
	}()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			streamCancel()
			_ = resp.Body.Close()
		})
	}
	return out, cancel, nil
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

func mapOpenCodeEventToCodex(raw string, sessionID string, usedPermissionIDs map[int]string) []types.CodexEvent {
	var event struct {
		Type       string         `json:"type"`
		Properties map[string]any `json:"properties"`
	}
	if err := json.Unmarshal([]byte(raw), &event); err != nil {
		return nil
	}
	eventType := strings.TrimSpace(strings.ToLower(event.Type))
	if eventType == "" {
		return nil
	}
	props := event.Properties
	if props == nil {
		props = map[string]any{}
	}
	now := time.Now().UTC()
	build := func(method string, id *int, payload map[string]any) types.CodexEvent {
		var params json.RawMessage
		if len(payload) > 0 {
			params, _ = json.Marshal(payload)
		}
		return types.CodexEvent{
			ID:     id,
			Method: method,
			Params: params,
			TS:     now.Format(time.RFC3339Nano),
		}
	}
	if sid := openCodeEventSessionID(eventType, props); sid != "" && sid != sessionID {
		return nil
	}

	switch eventType {
	case "session.status":
		status, _ := props["status"].(map[string]any)
		statusType := strings.ToLower(strings.TrimSpace(asString(status["type"])))
		switch statusType {
		case "busy":
			return []types.CodexEvent{build("turn/started", nil, map[string]any{
				"turn": map[string]any{"status": "in_progress"},
			})}
		case "idle":
			return []types.CodexEvent{build("turn/completed", nil, map[string]any{
				"turn": map[string]any{"status": "completed"},
			})}
		default:
			return nil
		}
	case "session.idle":
		return []types.CodexEvent{build("turn/completed", nil, map[string]any{
			"turn": map[string]any{"status": "completed"},
		})}
	case "session.error":
		errData := map[string]any{}
		if rawErr, ok := props["error"].(map[string]any); ok && rawErr != nil {
			errData = rawErr
		}
		msg := openCodeEventErrorMessage(errData)
		if msg == "" {
			msg = "session error"
		}
		events := []types.CodexEvent{
			build("error", nil, map[string]any{"error": map[string]any{"message": msg}}),
		}
		name := strings.ToLower(strings.TrimSpace(asString(errData["name"])))
		if name == "messageabortederror" {
			events = append(events, build("turn/completed", nil, map[string]any{
				"turn": map[string]any{"status": "interrupted"},
			}))
		}
		return events
	case "message.part.updated":
		part, _ := props["part"].(map[string]any)
		if part == nil {
			return nil
		}
		partType := strings.ToLower(strings.TrimSpace(asString(part["type"])))
		switch partType {
		case "step-start":
			item := map[string]any{
				"id":   strings.TrimSpace(asString(part["messageID"])),
				"type": "agentMessage",
			}
			return []types.CodexEvent{build("item/started", nil, map[string]any{"item": item})}
		case "step-finish":
			item := map[string]any{
				"id":   strings.TrimSpace(asString(part["messageID"])),
				"type": "agentMessage",
			}
			return []types.CodexEvent{build("item/completed", nil, map[string]any{"item": item})}
		case "text":
			delta := strings.TrimSpace(asString(props["delta"]))
			if delta == "" {
				delta = strings.TrimSpace(asString(part["text"]))
			}
			if delta == "" {
				return nil
			}
			return []types.CodexEvent{build("item/agentMessage/delta", nil, map[string]any{"delta": delta})}
		case "reasoning":
			text := strings.TrimSpace(asString(part["text"]))
			if text == "" {
				return nil
			}
			item := map[string]any{
				"id":   strings.TrimSpace(asString(part["id"])),
				"type": "reasoning",
				"text": text,
			}
			return []types.CodexEvent{build("item/updated", nil, map[string]any{"item": item})}
		default:
			return nil
		}
	case "permission.updated":
		permissionID := strings.TrimSpace(asString(props["id"]))
		if permissionID == "" {
			return nil
		}
		permission := openCodePermission{
			PermissionID: permissionID,
			SessionID:    strings.TrimSpace(asString(props["sessionID"])),
			Kind:         strings.TrimSpace(asString(props["type"])),
			Summary:      strings.TrimSpace(asString(props["title"])),
			CreatedAt:    openCodePermissionCreatedAt(props),
			Raw:          props,
		}
		metadata, _ := props["metadata"].(map[string]any)
		if metadata != nil {
			if permission.Command == "" {
				permission.Command = strings.TrimSpace(asString(metadata["command"]))
			}
			if permission.Command == "" {
				permission.Command = strings.TrimSpace(asString(metadata["parsedCmd"]))
			}
			if permission.Reason == "" {
				permission.Reason = strings.TrimSpace(asString(metadata["reason"]))
			}
		}
		method := openCodePermissionMethod(permission)
		requestID := openCodePermissionRequestID(permission.PermissionID, usedPermissionIDs)
		params := map[string]any{
			"permission_id": permission.PermissionID,
			"session_id":    permission.SessionID,
			"type":          permission.Kind,
			"title":         permission.Summary,
		}
		switch method {
		case "item/commandExecution/requestApproval":
			if permission.Command != "" {
				params["parsedCmd"] = permission.Command
			}
		case "item/fileChange/requestApproval":
			if permission.Reason != "" {
				params["reason"] = permission.Reason
			}
		default:
			if permission.Summary != "" {
				params["questions"] = []map[string]any{
					{"text": permission.Summary},
				}
			}
		}
		return []types.CodexEvent{build(method, &requestID, params)}
	case "permission.replied":
		permissionID := strings.TrimSpace(asString(props["permissionID"]))
		if permissionID == "" {
			return nil
		}
		requestID := openCodePermissionRequestID(permissionID, usedPermissionIDs)
		return []types.CodexEvent{build("permission/replied", &requestID, map[string]any{
			"permission_id": permissionID,
			"request_id":    requestID,
			"response":      strings.TrimSpace(asString(props["response"])),
		})}
	default:
		return nil
	}
}

func openCodeEventSessionID(eventType string, properties map[string]any) string {
	if properties == nil {
		return ""
	}
	switch eventType {
	case "session.status", "session.idle", "session.compacted", "session.error":
		return strings.TrimSpace(asString(properties["sessionID"]))
	case "message.updated":
		info, _ := properties["info"].(map[string]any)
		return strings.TrimSpace(asString(info["sessionID"]))
	case "message.part.updated", "message.part.removed":
		part, _ := properties["part"].(map[string]any)
		return strings.TrimSpace(asString(part["sessionID"]))
	case "permission.updated", "permission.replied":
		return strings.TrimSpace(asString(properties["sessionID"]))
	default:
		return ""
	}
}

func openCodeEventErrorMessage(raw map[string]any) string {
	if raw == nil {
		return ""
	}
	if msg := strings.TrimSpace(asString(raw["message"])); msg != "" {
		return msg
	}
	data, _ := raw["data"].(map[string]any)
	if data != nil {
		if msg := strings.TrimSpace(asString(data["message"])); msg != "" {
			return msg
		}
	}
	return ""
}

func openCodePermissionCreatedAt(raw map[string]any) time.Time {
	if raw == nil {
		return time.Time{}
	}
	if when := openCodeTimestamp(raw["createdAt"]); !when.IsZero() {
		return when
	}
	if when := openCodeTimestamp(raw["ts"]); !when.IsZero() {
		return when
	}
	if clock, ok := raw["time"].(map[string]any); ok && clock != nil {
		if when := openCodeTimestamp(clock["created"]); !when.IsZero() {
			return when
		}
	}
	return time.Time{}
}

func openCodeModelID(providerID string, entry any) string {
	switch value := entry.(type) {
	case string:
		return openCodeNormalizedModelID(providerID, value)
	case map[string]any:
		modelID := strings.TrimSpace(asString(value["id"]))
		if modelID == "" {
			modelID = strings.TrimSpace(asString(value["modelID"]))
		}
		return openCodeNormalizedModelID(providerID, modelID)
	default:
		return ""
	}
}

func openCodeRawModelID(entry any) string {
	switch value := entry.(type) {
	case string:
		return strings.TrimSpace(value)
	case map[string]any:
		modelID := strings.TrimSpace(asString(value["id"]))
		if modelID == "" {
			modelID = strings.TrimSpace(asString(value["modelID"]))
		}
		return modelID
	default:
		return ""
	}
}

func openCodeModelEntries(raw any) []any {
	switch typed := raw.(type) {
	case []any:
		return typed
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		out := make([]any, 0, len(keys))
		for _, key := range keys {
			entry := typed[key]
			if mapped, ok := entry.(map[string]any); ok {
				modelID := strings.TrimSpace(asString(mapped["id"]))
				if modelID == "" {
					modelID = strings.TrimSpace(asString(mapped["modelID"]))
				}
				if modelID == "" {
					cloned := make(map[string]any, len(mapped)+1)
					for k, v := range mapped {
						cloned[k] = v
					}
					cloned["id"] = key
					out = append(out, cloned)
					continue
				}
				out = append(out, mapped)
				continue
			}
			if modelID := strings.TrimSpace(asString(entry)); modelID != "" {
				out = append(out, modelID)
				continue
			}
			out = append(out, map[string]any{"id": key})
		}
		return out
	default:
		return nil
	}
}

func openCodeNormalizedModelID(providerID, modelID string) string {
	providerID = strings.TrimSpace(providerID)
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return ""
	}
	if providerID == "" {
		return modelID
	}
	if strings.HasPrefix(modelID, providerID+"/") {
		return modelID
	}
	return providerID + "/" + modelID
}

func openCodeParseProviderCatalog(providers []map[string]any, defaults map[string]any) openCodeParsedProviderCatalog {
	out := openCodeParsedProviderCatalog{
		ProviderIDs:      map[string]struct{}{},
		ModelToProvider:  map[string]string{},
		NormalizedModels: []string{},
	}
	seen := map[string]struct{}{}
	for _, provider := range providers {
		if provider == nil {
			continue
		}
		providerID := strings.TrimSpace(asString(provider["id"]))
		if providerID == "" {
			providerID = strings.TrimSpace(asString(provider["providerID"]))
		}
		if providerID == "" {
			continue
		}
		out.ProviderIDs[providerID] = struct{}{}
		for _, entry := range openCodeModelEntries(provider["models"]) {
			rawModelID := openCodeRawModelID(entry)
			if rawModelID != "" {
				if _, exists := out.ModelToProvider[rawModelID]; !exists {
					out.ModelToProvider[rawModelID] = providerID
				}
			}
			modelID := openCodeModelID(providerID, entry)
			if modelID == "" {
				continue
			}
			if _, exists := seen[modelID]; exists {
				continue
			}
			seen[modelID] = struct{}{}
			out.NormalizedModels = append(out.NormalizedModels, modelID)
		}
		if out.DefaultModel == "" {
			if value, ok := defaults[providerID]; ok {
				out.DefaultModel = openCodeNormalizedModelID(providerID, strings.TrimSpace(asString(value)))
			}
		}
	}
	if out.DefaultModel != "" {
		sort.SliceStable(out.NormalizedModels, func(i, j int) bool {
			left := out.NormalizedModels[i]
			right := out.NormalizedModels[j]
			if left == out.DefaultModel {
				return true
			}
			if right == out.DefaultModel {
				return false
			}
			return i < j
		})
	}
	return out
}

func extractOpenCodePartsText(parts []map[string]any) string {
	if len(parts) == 0 {
		return ""
	}
	texts := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == nil {
			continue
		}
		typ := strings.ToLower(strings.TrimSpace(asString(part["type"])))
		if typ != "" && typ != "text" {
			continue
		}
		text := strings.TrimSpace(asString(part["text"]))
		if text == "" {
			continue
		}
		texts = append(texts, text)
	}
	return strings.TrimSpace(strings.Join(texts, "\n"))
}

func extractOpenCodeSessionMessageText(message openCodeSessionMessage) string {
	if text := extractOpenCodePartsText(message.Parts); text != "" {
		return text
	}
	if message.Message != nil {
		if text := strings.TrimSpace(extractClaudeMessageText(message.Message)); text != "" {
			return text
		}
	}
	return ""
}

func normalizeOpenCodeSessionMessages(payload any) []openCodeSessionMessage {
	switch typed := payload.(type) {
	case []any:
		return toOpenCodeSessionMessageSlice(typed)
	case map[string]any:
		for _, key := range []string{"messages", "items", "data"} {
			if list, ok := typed[key].([]any); ok {
				return toOpenCodeSessionMessageSlice(list)
			}
		}
		if parsed, ok := parseOpenCodeSessionMessage(typed); ok {
			return []openCodeSessionMessage{parsed}
		}
	default:
		return nil
	}
	return nil
}

func toOpenCodeSessionMessageSlice(values []any) []openCodeSessionMessage {
	out := make([]openCodeSessionMessage, 0, len(values))
	for _, value := range values {
		entry, ok := value.(map[string]any)
		if !ok || entry == nil {
			continue
		}
		parsed, ok := parseOpenCodeSessionMessage(entry)
		if !ok {
			continue
		}
		out = append(out, parsed)
	}
	return out
}

func parseOpenCodeSessionMessage(raw map[string]any) (openCodeSessionMessage, bool) {
	if raw == nil {
		return openCodeSessionMessage{}, false
	}
	info, _ := raw["info"].(map[string]any)
	message, _ := raw["message"].(map[string]any)
	if info == nil {
		info = map[string]any{}
		if role := strings.TrimSpace(asString(raw["role"])); role != "" {
			info["role"] = role
		}
		if id := strings.TrimSpace(asString(raw["id"])); id != "" {
			info["id"] = id
		}
		if created := raw["createdAt"]; created != nil {
			info["createdAt"] = created
		}
	}
	parts := toOpenCodeMapSlice(openCodeModelEntries(raw["parts"]))
	if len(parts) == 0 && message != nil {
		parts = toOpenCodeMapSlice(openCodeModelEntries(message["content"]))
	}
	if len(parts) == 0 && len(info) == 0 && message == nil {
		return openCodeSessionMessage{}, false
	}
	return openCodeSessionMessage{
		Info:    info,
		Parts:   parts,
		Message: message,
	}, true
}

func openCodeLatestAssistantSnapshot(messages []openCodeSessionMessage) openCodeAssistantSnapshot {
	var (
		best      openCodeAssistantSnapshot
		bestSet   bool
		bestIndex = -1
	)
	for i, message := range messages {
		role := strings.ToLower(strings.TrimSpace(openCodeSessionMessageRole(message)))
		if role != "assistant" && role != "model" {
			continue
		}
		text := extractOpenCodeSessionMessageText(message)
		if text == "" {
			continue
		}
		candidate := openCodeAssistantSnapshot{
			MessageID: openCodeSessionMessageID(message),
			Text:      text,
			CreatedAt: openCodeSessionMessageCreatedAt(message),
		}
		if !bestSet {
			best = candidate
			bestSet = true
			bestIndex = i
			continue
		}
		if candidate.CreatedAt.After(best.CreatedAt) {
			best = candidate
			bestIndex = i
			continue
		}
		if candidate.CreatedAt.Equal(best.CreatedAt) && i > bestIndex {
			best = candidate
			bestIndex = i
		}
	}
	return best
}

func openCodeSessionMessageRole(message openCodeSessionMessage) string {
	if message.Info != nil {
		if role := strings.TrimSpace(asString(message.Info["role"])); role != "" {
			return role
		}
		if role := strings.TrimSpace(asString(message.Info["type"])); role != "" {
			return role
		}
	}
	if message.Message != nil {
		if role := strings.TrimSpace(asString(message.Message["role"])); role != "" {
			return role
		}
	}
	return ""
}

func openCodeSessionMessageID(message openCodeSessionMessage) string {
	if message.Info != nil {
		if id := strings.TrimSpace(asString(message.Info["id"])); id != "" {
			return id
		}
		if id := strings.TrimSpace(asString(message.Info["messageID"])); id != "" {
			return id
		}
	}
	if message.Message != nil {
		if id := strings.TrimSpace(asString(message.Message["id"])); id != "" {
			return id
		}
	}
	return ""
}

func openCodeSessionMessageCreatedAt(message openCodeSessionMessage) time.Time {
	if message.Info != nil {
		if when := openCodeTimestamp(message.Info["createdAt"]); !when.IsZero() {
			return when
		}
		if when := openCodeTimestamp(message.Info["ts"]); !when.IsZero() {
			return when
		}
	}
	if message.Message != nil {
		if when := openCodeTimestamp(message.Message["createdAt"]); !when.IsZero() {
			return when
		}
	}
	return time.Time{}
}

func openCodeAssistantChanged(current, baseline openCodeAssistantSnapshot) bool {
	if strings.TrimSpace(current.Text) == "" {
		return false
	}
	currentID := strings.TrimSpace(current.MessageID)
	baselineID := strings.TrimSpace(baseline.MessageID)
	if currentID != "" && baselineID != "" {
		return currentID != baselineID
	}
	if currentID != "" && baselineID == "" {
		return true
	}
	if baseline.Text == "" {
		return true
	}
	return current.Text != baseline.Text
}

func normalizeOpenCodePermissionList(payload any) []map[string]any {
	switch typed := payload.(type) {
	case []any:
		return toOpenCodeMapSlice(typed)
	case map[string]any:
		for _, key := range []string{"permissions", "data", "items"} {
			if list, ok := typed[key].([]any); ok {
				return toOpenCodeMapSlice(list)
			}
		}
		return []map[string]any{typed}
	default:
		return nil
	}
}

func toOpenCodeMapSlice(values []any) []map[string]any {
	out := make([]map[string]any, 0, len(values))
	for _, value := range values {
		entry, ok := value.(map[string]any)
		if !ok || entry == nil {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func parseOpenCodePermission(item map[string]any) (openCodePermission, bool) {
	if item == nil {
		return openCodePermission{}, false
	}
	permissionID := strings.TrimSpace(asString(item["id"]))
	if permissionID == "" {
		permissionID = strings.TrimSpace(asString(item["permissionID"]))
	}
	if permissionID == "" {
		return openCodePermission{}, false
	}

	sessionID := strings.TrimSpace(asString(item["sessionID"]))
	if sessionID == "" {
		sessionID = strings.TrimSpace(asString(item["sessionId"]))
	}
	if sessionID == "" {
		if session, ok := item["session"].(map[string]any); ok {
			sessionID = strings.TrimSpace(asString(session["id"]))
		}
	}

	status := strings.ToLower(strings.TrimSpace(asString(item["status"])))
	kind := strings.TrimSpace(asString(item["type"]))
	if kind == "" {
		kind = strings.TrimSpace(asString(item["kind"]))
	}
	summary := strings.TrimSpace(asString(item["message"]))
	command := strings.TrimSpace(asString(item["command"]))
	if command == "" {
		command = strings.TrimSpace(asString(item["parsedCmd"]))
	}
	reason := strings.TrimSpace(asString(item["reason"]))
	createdAt := openCodeTimestamp(item["createdAt"])
	if createdAt.IsZero() {
		createdAt = openCodeTimestamp(item["ts"])
	}
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	return openCodePermission{
		PermissionID: permissionID,
		SessionID:    sessionID,
		Status:       status,
		Kind:         kind,
		Summary:      summary,
		Command:      command,
		Reason:       reason,
		CreatedAt:    createdAt,
		Raw:          item,
	}, true
}

func openCodeTimestamp(raw any) time.Time {
	switch value := raw.(type) {
	case string:
		if parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(value)); err == nil {
			return parsed.UTC()
		}
		if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value)); err == nil {
			return parsed.UTC()
		}
	case float64:
		return time.Unix(int64(value), 0).UTC()
	case int64:
		return time.Unix(value, 0).UTC()
	case int:
		return time.Unix(int64(value), 0).UTC()
	case json.Number:
		if i, err := strconv.ParseInt(value.String(), 10, 64); err == nil {
			return time.Unix(i, 0).UTC()
		}
	}
	return time.Time{}
}

func openCodePermissionMethod(permission openCodePermission) string {
	kind := strings.ToLower(strings.TrimSpace(permission.Kind))
	switch {
	case strings.Contains(kind, "command"), strings.Contains(kind, "exec"), strings.Contains(kind, "shell"):
		return "item/commandExecution/requestApproval"
	case strings.Contains(kind, "file"), strings.Contains(kind, "write"), strings.Contains(kind, "edit"):
		return "item/fileChange/requestApproval"
	default:
		return "tool/requestUserInput"
	}
}

func openCodePermissionRequestID(permissionID string, used map[int]string) int {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(strings.TrimSpace(permissionID)))
	base := int(hash.Sum32() & 0x7fffffff)
	if base == 0 {
		base = 1
	}
	candidate := base
	for {
		if existing, ok := used[candidate]; !ok || existing == permissionID {
			used[candidate] = permissionID
			return candidate
		}
		candidate++
		if candidate <= 0 {
			candidate = 1
		}
	}
}

func normalizeApprovalDecision(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "accept", "approved", "allow", "yes":
		return "accept"
	case "decline", "deny", "rejected", "no":
		return "decline"
	default:
		return value
	}
}

func normalizeOpenCodePermissionResponse(raw string) string {
	switch normalizeApprovalDecision(raw) {
	case "accept":
		return "once"
	case "decline":
		return "reject"
	default:
		return "once"
	}
}
