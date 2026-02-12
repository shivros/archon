package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"control/internal/types"
)

type openCodeClient struct {
	baseURL    string
	username   string
	token      string
	httpClient *http.Client
}

type openCodeClientConfig struct {
	BaseURL  string
	Username string
	Token    string
	Timeout  time.Duration
}

type openCodeModelCatalog struct {
	Models       []string
	DefaultModel string
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
	return &openCodeClient{
		baseURL:  strings.TrimRight(parsed.String(), "/"),
		username: username,
		token:    strings.TrimSpace(cfg.Token),
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}, nil
}

func (c *openCodeClient) CreateSession(ctx context.Context, title string) (string, error) {
	payload := map[string]any{}
	if strings.TrimSpace(title) != "" {
		payload["title"] = strings.TrimSpace(title)
	}
	var result struct {
		ID string `json:"id"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/session", payload, &result); err != nil {
		return "", err
	}
	if strings.TrimSpace(result.ID) == "" {
		return "", fmt.Errorf("session id missing from server response")
	}
	return strings.TrimSpace(result.ID), nil
}

func (c *openCodeClient) Prompt(ctx context.Context, sessionID, text string, runtimeOptions *types.SessionRuntimeOptions) (string, error) {
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
	if model := openCodeModelFromRuntime(runtimeOptions); len(model) > 0 {
		body["model"] = model
	}

	var result struct {
		Parts []map[string]any `json:"parts"`
	}
	path := fmt.Sprintf("/session/%s/prompt", url.PathEscape(sessionID))
	if err := c.doJSON(ctx, http.MethodPost, path, body, &result); err != nil {
		return "", err
	}
	return extractOpenCodePartsText(result.Parts), nil
}

func (c *openCodeClient) AbortSession(ctx context.Context, sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return fmt.Errorf("session id is required")
	}
	path := fmt.Sprintf("/session/%s/abort", url.PathEscape(sessionID))
	return c.doJSON(ctx, http.MethodPost, path, map[string]any{}, nil)
}

func (c *openCodeClient) ListModels(ctx context.Context) (*openCodeModelCatalog, error) {
	var payload struct {
		Providers []map[string]any `json:"providers"`
		Default   map[string]any   `json:"default"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/config/providers", nil, &payload); err != nil {
		return nil, err
	}

	out := &openCodeModelCatalog{}
	seen := map[string]struct{}{}
	defaults := payload.Default

	for _, provider := range payload.Providers {
		if provider == nil {
			continue
		}
		providerID := strings.TrimSpace(asString(provider["id"]))
		if providerID == "" {
			providerID = strings.TrimSpace(asString(provider["providerID"]))
		}
		models, _ := provider["models"].([]any)
		for _, entry := range models {
			modelID := openCodeModelID(providerID, entry)
			if modelID == "" {
				continue
			}
			if _, ok := seen[modelID]; ok {
				continue
			}
			seen[modelID] = struct{}{}
			out.Models = append(out.Models, modelID)
		}
		if out.DefaultModel == "" {
			if value, ok := defaults[providerID]; ok {
				out.DefaultModel = openCodeNormalizedModelID(providerID, strings.TrimSpace(asString(value)))
			}
		}
	}
	if out.DefaultModel != "" {
		sort.SliceStable(out.Models, func(i, j int) bool {
			left := out.Models[i]
			right := out.Models[j]
			if left == out.DefaultModel {
				return true
			}
			if right == out.DefaultModel {
				return false
			}
			return i < j
		})
	}
	return out, nil
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
		return fmt.Errorf("opencode request failed (%s %s): %s", method, path, msg)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func openCodeModelFromRuntime(runtimeOptions *types.SessionRuntimeOptions) map[string]string {
	if runtimeOptions == nil {
		return nil
	}
	raw := strings.TrimSpace(runtimeOptions.Model)
	if raw == "" {
		return nil
	}
	if strings.Contains(raw, "/") {
		parts := strings.SplitN(raw, "/", 2)
		providerID := strings.TrimSpace(parts[0])
		modelID := strings.TrimSpace(parts[1])
		if providerID != "" && modelID != "" {
			return map[string]string{
				"providerID": providerID,
				"modelID":    modelID,
			}
		}
	}
	return map[string]string{"modelID": raw}
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

func openCodeNormalizedModelID(providerID, modelID string) string {
	providerID = strings.TrimSpace(providerID)
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return ""
	}
	if strings.Contains(modelID, "/") || providerID == "" {
		return modelID
	}
	return providerID + "/" + modelID
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
