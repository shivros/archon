package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"control/internal/types"
)

var errOpenCodePromptPending = errors.New("opencode prompt pending")

type openCodeJSONRequester interface {
	doJSON(ctx context.Context, method, path string, body any, out any) error
}

type openCodeRuntimeModelProvider interface {
	resolveRuntimeModel(ctx context.Context, runtimeOptions *types.SessionRuntimeOptions) map[string]string
}

type openCodeSessionService struct {
	requester openCodeJSONRequester
}

type openCodePromptService struct {
	requester     openCodeJSONRequester
	sessions      *openCodeSessionService
	modelProvider openCodeRuntimeModelProvider
}

type openCodeCatalogService struct {
	requester openCodeJSONRequester

	catalogMu         sync.RWMutex
	catalogFetchedAt  time.Time
	catalogProviders  map[string]struct{}
	catalogModelToPID map[string]string
}

type openCodePermissionService struct {
	requester openCodeJSONRequester
}

type openCodeEventService struct {
	baseURL   string
	username  string
	token     string
	transport http.RoundTripper
}

func newOpenCodeSessionService(requester openCodeJSONRequester) *openCodeSessionService {
	return &openCodeSessionService{requester: requester}
}

func newOpenCodePromptService(requester openCodeJSONRequester, sessions *openCodeSessionService, modelProvider openCodeRuntimeModelProvider) *openCodePromptService {
	return &openCodePromptService{
		requester:     requester,
		sessions:      sessions,
		modelProvider: modelProvider,
	}
}

func newOpenCodeCatalogService(requester openCodeJSONRequester) *openCodeCatalogService {
	return &openCodeCatalogService{requester: requester}
}

func newOpenCodePermissionService(requester openCodeJSONRequester) *openCodePermissionService {
	return &openCodePermissionService{requester: requester}
}

func newOpenCodeEventService(baseURL, username, token string, transport http.RoundTripper) *openCodeEventService {
	return &openCodeEventService{
		baseURL:   strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		username:  strings.TrimSpace(username),
		token:     strings.TrimSpace(token),
		transport: transport,
	}
}

func (s *openCodeSessionService) CreateSession(ctx context.Context, title, directory string) (string, error) {
	if s == nil || s.requester == nil {
		return "", errors.New("requester is required")
	}
	startedAt := time.Now().Add(-2 * time.Second)
	payload := map[string]any{}
	if strings.TrimSpace(title) != "" {
		payload["title"] = strings.TrimSpace(title)
	}
	var result map[string]any
	path := appendOpenCodeDirectoryQuery("/session", directory)
	if err := s.requester.doJSON(ctx, http.MethodPost, path, payload, &result); err != nil {
		if errors.Is(err, io.EOF) {
			return s.lookupCreatedSessionID(ctx, title, directory, startedAt)
		}
		return "", err
	}
	if sessionID := openCodeExtractSessionID(result); sessionID != "" {
		return sessionID, nil
	}
	sessionID, lookupErr := s.lookupCreatedSessionID(ctx, title, directory, startedAt)
	if lookupErr == nil && strings.TrimSpace(sessionID) != "" {
		return sessionID, nil
	}
	if lookupErr != nil {
		return "", lookupErr
	}
	return "", fmt.Errorf("session id missing from server response")
}

func (s *openCodeSessionService) lookupCreatedSessionID(ctx context.Context, title, directory string, startedAt time.Time) (string, error) {
	if s == nil || s.requester == nil {
		return "", errors.New("requester is required")
	}
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
	if err := s.requester.doJSON(ctx, http.MethodGet, path, nil, &sessions); err != nil {
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

	var newestID string
	var newestCreated time.Time
	for _, session := range sessions {
		id := openCodeExtractSessionID(session)
		if id == "" {
			continue
		}
		createdAt := openCodeSessionCreatedAt(session)
		if !startedAt.IsZero() && createdAt.IsZero() {
			continue
		}
		if !startedAt.IsZero() && createdAt.Before(startedAt) {
			continue
		}
		if newestID == "" || createdAt.After(newestCreated) {
			newestID = id
			newestCreated = createdAt
		}
	}
	if newestID != "" {
		return newestID, nil
	}

	if title == "" {
		return "", fmt.Errorf("session id missing from server response")
	}
	for _, session := range sessions {
		if id := openCodeExtractSessionID(session); id != "" {
			return id, nil
		}
	}
	return "", fmt.Errorf("session id missing from server response")
}

func (s *openCodeSessionService) AbortSession(ctx context.Context, sessionID, directory string) error {
	if s == nil || s.requester == nil {
		return errors.New("requester is required")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return fmt.Errorf("session id is required")
	}
	path := appendOpenCodeDirectoryQuery(fmt.Sprintf("/session/%s/abort", url.PathEscape(sessionID)), directory)
	return s.requester.doJSON(ctx, http.MethodPost, path, map[string]any{}, nil)
}

func (s *openCodeSessionService) listSessionMessages(ctx context.Context, sessionID, directory string, limit int) ([]openCodeSessionMessage, error) {
	if s == nil || s.requester == nil {
		return nil, errors.New("requester is required")
	}
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
	if err := s.requester.doJSON(ctx, http.MethodGet, pathWithDirectory, nil, &payload); err != nil {
		if strings.TrimSpace(directory) == "" || !openCodeShouldFallbackLegacy(err) {
			return nil, err
		}
		payload = nil
		if retryErr := s.requester.doJSON(ctx, http.MethodGet, path, nil, &payload); retryErr != nil {
			return nil, retryErr
		}
	}
	return normalizeOpenCodeSessionMessages(payload), nil
}

func (s *openCodeSessionService) latestAssistantSnapshot(ctx context.Context, sessionID, directory string) openCodeAssistantSnapshot {
	messages, err := s.listSessionMessages(ctx, sessionID, directory, 8)
	if err != nil {
		return openCodeAssistantSnapshot{}
	}
	return openCodeLatestAssistantSnapshot(messages)
}

func (s *openCodeSessionService) waitForAssistantReply(ctx context.Context, sessionID, directory string, baseline openCodeAssistantSnapshot) string {
	deadline := time.Now().Add(12 * time.Second)
	delay := 250 * time.Millisecond
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return ""
		}
		messages, err := s.listSessionMessages(ctx, sessionID, directory, 20)
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

func (s *openCodePromptService) Prompt(ctx context.Context, sessionID, text string, runtimeOptions *types.SessionRuntimeOptions, directory string) (string, error) {
	if s == nil || s.requester == nil || s.sessions == nil {
		return "", errors.New("prompt service dependencies are required")
	}
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
	if s.modelProvider != nil {
		if model := s.modelProvider.resolveRuntimeModel(ctx, runtimeOptions); len(model) > 0 {
			body["model"] = model
		}
	}

	baseline := s.sessions.latestAssistantSnapshot(ctx, sessionID, directory)

	var result openCodeSessionMessage
	paths := []string{
		appendOpenCodeDirectoryQuery(fmt.Sprintf("/session/%s/message", url.PathEscape(sessionID)), directory),
		appendOpenCodeDirectoryQuery(fmt.Sprintf("/session/%s/prompt", url.PathEscape(sessionID)), directory),
	}
	var lastErr error
	for idx, path := range paths {
		err := s.requester.doJSON(ctx, http.MethodPost, path, body, &result)
		if err == nil {
			reply := extractOpenCodeSessionMessageText(result)
			if reply != "" {
				return reply, nil
			}
			if recovered := s.sessions.waitForAssistantReply(ctx, sessionID, directory, baseline); recovered != "" {
				return recovered, nil
			}
			return "", nil
		}
		if errors.Is(err, io.EOF) {
			if recovered := s.sessions.waitForAssistantReply(ctx, sessionID, directory, baseline); recovered != "" {
				return recovered, nil
			}
			return "", nil
		}
		if openCodeRequestTimedOut(err) {
			// Some OpenCode/Kilo server builds keep processing after the client-side
			// HTTP timeout. Recover the assistant turn from session history when
			// possible instead of surfacing a false-negative send failure.
			recoveryCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			recovered := s.sessions.waitForAssistantReply(recoveryCtx, sessionID, directory, baseline)
			cancel()
			if recovered != "" {
				return recovered, nil
			}
			return "", fmt.Errorf("%w: %v", errOpenCodePromptPending, err)
		}
		lastErr = err
		if idx == 0 && openCodeShouldFallbackLegacy(err) {
			continue
		}
		return "", err
	}
	return "", lastErr
}

func openCodeRequestTimedOut(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func (s *openCodeCatalogService) openCodeModelCatalogIndex(ctx context.Context) (map[string]struct{}, map[string]string, error) {
	if s == nil || s.requester == nil {
		return nil, nil, errors.New("requester is required")
	}
	s.catalogMu.RLock()
	if s.catalogProviders != nil && s.catalogModelToPID != nil && time.Since(s.catalogFetchedAt) < 30*time.Second {
		providers := cloneStringSet(s.catalogProviders)
		modelToProvider := cloneStringMap(s.catalogModelToPID)
		s.catalogMu.RUnlock()
		return providers, modelToProvider, nil
	}
	s.catalogMu.RUnlock()

	var payload struct {
		Providers []map[string]any `json:"providers"`
		Default   map[string]any   `json:"default"`
	}
	if err := s.requester.doJSON(ctx, http.MethodGet, "/config/providers", nil, &payload); err != nil {
		return nil, nil, err
	}
	parsed := openCodeParseProviderCatalog(payload.Providers, payload.Default)

	s.catalogMu.Lock()
	s.catalogProviders = cloneStringSet(parsed.ProviderIDs)
	s.catalogModelToPID = cloneStringMap(parsed.ModelToProvider)
	s.catalogFetchedAt = time.Now()
	s.catalogMu.Unlock()
	return parsed.ProviderIDs, parsed.ModelToProvider, nil
}

func (s *openCodeCatalogService) ListModels(ctx context.Context) (*openCodeModelCatalog, error) {
	if s == nil || s.requester == nil {
		return nil, errors.New("requester is required")
	}
	var payload struct {
		Providers []map[string]any `json:"providers"`
		Default   map[string]any   `json:"default"`
	}
	if err := s.requester.doJSON(ctx, http.MethodGet, "/config/providers", nil, &payload); err != nil {
		return nil, err
	}

	parsed := openCodeParseProviderCatalog(payload.Providers, payload.Default)
	return &openCodeModelCatalog{
		Models:       parsed.NormalizedModels,
		DefaultModel: parsed.DefaultModel,
	}, nil
}

func (s *openCodePermissionService) ListPermissions(ctx context.Context, sessionID, directory string) ([]openCodePermission, error) {
	if s == nil || s.requester == nil {
		return nil, errors.New("requester is required")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("session id is required")
	}

	query := url.Values{}
	query.Set("sessionID", sessionID)
	path := "/permission?" + query.Encode()
	pathWithDirectory := appendOpenCodeDirectoryQuery(path, directory)

	var payload any
	if err := s.requester.doJSON(ctx, http.MethodGet, pathWithDirectory, nil, &payload); err != nil {
		if strings.TrimSpace(directory) == "" || !openCodeShouldFallbackLegacy(err) {
			return nil, err
		}
		payload = nil
		if retryErr := s.requester.doJSON(ctx, http.MethodGet, path, nil, &payload); retryErr != nil {
			return nil, retryErr
		}
	}

	raw := normalizeOpenCodePermissionList(payload)
	if len(raw) == 0 {
		return []openCodePermission{}, nil
	}
	out := make([]openCodePermission, 0, len(raw))
	for _, item := range raw {
		permission, ok := parseOpenCodePermission(item)
		if !ok {
			continue
		}
		if permission.SessionID != "" && !strings.EqualFold(permission.SessionID, sessionID) {
			continue
		}
		if permission.Status != "" && permission.Status != "pending" {
			continue
		}
		out = append(out, permission)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (s *openCodePermissionService) ReplyPermission(ctx context.Context, sessionID, permissionID, decision string, responses []string, directory string) error {
	if s == nil || s.requester == nil {
		return errors.New("requester is required")
	}
	permissionID = strings.TrimSpace(permissionID)
	if permissionID == "" {
		return fmt.Errorf("permission id is required")
	}
	sessionID = strings.TrimSpace(sessionID)
	decision = strings.TrimSpace(decision)
	if decision == "" {
		decision = "approve"
	}

	legacyPath := appendOpenCodeDirectoryQuery(fmt.Sprintf("/permission/%s/reply", url.PathEscape(permissionID)), directory)
	legacyBody := map[string]any{"decision": decision}

	if sessionID == "" {
		return s.requester.doJSON(ctx, http.MethodPost, legacyPath, legacyBody, nil)
	}

	sessionPath := appendOpenCodeDirectoryQuery(
		fmt.Sprintf("/session/%s/permissions/%s", url.PathEscape(sessionID), url.PathEscape(permissionID)),
		directory,
	)
	sessionBody := map[string]any{
		"response": normalizeOpenCodePermissionResponse(decision),
	}
	if len(responses) > 0 {
		sessionBody["responses"] = append([]string(nil), responses...)
	}
	var sessionReply any
	if err := s.requester.doJSON(ctx, http.MethodPost, sessionPath, sessionBody, &sessionReply); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		if !openCodeShouldFallbackLegacy(err) && !openCodeShouldFallbackLegacySessionPermissionReply(err) {
			return err
		}
		return s.requester.doJSON(ctx, http.MethodPost, legacyPath, legacyBody, nil)
	}
	return nil
}

func openCodeShouldFallbackLegacySessionPermissionReply(err error) bool {
	if err == nil {
		return false
	}
	// Some OpenCode/Kilo builds serve SPA HTML for unknown API routes with HTTP 200.
	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		return true
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(message, "invalid character '<'")
}

func (s *openCodeEventService) SubscribeSessionEvents(ctx context.Context, sessionID, directory string) (<-chan types.CodexEvent, func(), error) {
	if s == nil {
		return nil, nil, errors.New("event service is required")
	}
	if strings.TrimSpace(s.baseURL) == "" {
		return nil, nil, errors.New("base url is required")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, nil, fmt.Errorf("session id is required")
	}

	query := url.Values{}
	query.Set("parentID", sessionID)
	path := "/event?" + query.Encode()
	pathWithDirectory := appendOpenCodeDirectoryQuery(path, directory)
	endpoint := s.baseURL + pathWithDirectory

	streamCtx, cancel := context.WithCancel(ctx)
	req, err := http.NewRequestWithContext(streamCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	req.Header.Set("Accept", "text/event-stream")
	if s.token != "" {
		req.SetBasicAuth(s.username, s.token)
	}

	streamClient := &http.Client{Transport: s.transport}
	resp, err := streamClient.Do(req)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		cancel()
		msg := strings.TrimSpace(string(raw))
		if msg == "" {
			msg = resp.Status
		}
		return nil, nil, &openCodeRequestError{
			Method:     http.MethodGet,
			Path:       pathWithDirectory,
			StatusCode: resp.StatusCode,
			Message:    msg,
		}
	}
	contentType := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
	if !strings.Contains(contentType, "text/event-stream") {
		_ = resp.Body.Close()
		cancel()
		return nil, nil, fmt.Errorf("opencode event stream failed: unexpected content type %q", contentType)
	}

	events := make(chan types.CodexEvent, 32)
	var closeOnce sync.Once
	closeFn := func() {
		closeOnce.Do(func() {
			cancel()
			_ = resp.Body.Close()
		})
	}

	go func() {
		defer closeFn()
		defer close(events)

		scanner := bufio.NewScanner(resp.Body)
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 1024*1024)

		usedPermissionIDs := map[int]string{}
		dataLines := make([]string, 0, 4)
		emit := func(payload string) bool {
			for _, event := range mapOpenCodeEventToCodex(payload, sessionID, usedPermissionIDs) {
				select {
				case <-streamCtx.Done():
					return false
				case events <- event:
				}
			}
			return true
		}

		for scanner.Scan() {
			line := scanner.Text()
			switch {
			case strings.HasPrefix(line, ":"):
				continue
			case strings.HasPrefix(line, "data:"):
				dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			case strings.TrimSpace(line) == "":
				if len(dataLines) == 0 {
					continue
				}
				payload := strings.TrimSpace(strings.Join(dataLines, "\n"))
				dataLines = dataLines[:0]
				if payload == "" {
					continue
				}
				if !emit(payload) {
					return
				}
			}
		}

		if len(dataLines) > 0 {
			payload := strings.TrimSpace(strings.Join(dataLines, "\n"))
			if payload != "" {
				_ = emit(payload)
			}
		}

		if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, context.Canceled) {
			errEvent := types.CodexEvent{
				Method: "error",
				TS:     time.Now().UTC().Format(time.RFC3339Nano),
			}
			params, _ := json.Marshal(map[string]any{
				"error": map[string]any{
					"message": fmt.Sprintf("opencode event stream failed: %v", err),
				},
			})
			errEvent.Params = params
			select {
			case <-streamCtx.Done():
			case events <- errEvent:
			}
		}
	}()

	return events, closeFn, nil
}
