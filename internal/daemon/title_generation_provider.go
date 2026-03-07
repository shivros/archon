package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"control/internal/config"
	"control/internal/logging"
)

type titleGenerationProviderFactory interface {
	Build(coreCfg titleGenerationProviderConfigResolver) (titleGenerator, bool, error)
}

type titleGenerationProviderConfigResolver interface {
	TitleGenerationProvider() string
	TitleGenerationModel() string
	TitleGenerationTimeoutSeconds() int
	TitleGenerationOpenRouterAPIKey() string
	TitleGenerationOpenRouterAPIKeyEnv() string
	TitleGenerationOpenRouterBaseURL() string
}

type titleProviderBridge struct {
	factories map[string]titleGenerationProviderFactory
	logger    logging.Logger
}

func newTitleProviderBridge(logger logging.Logger) *titleProviderBridge {
	if logger == nil {
		logger = logging.Nop()
	}
	return &titleProviderBridge{
		factories: map[string]titleGenerationProviderFactory{
			"openrouter": openRouterTitleGeneratorFactory{},
		},
		logger: logger,
	}
}

func (b *titleProviderBridge) Build(coreCfg titleGenerationProviderConfigResolver) (titleGenerator, bool, error) {
	if b == nil || coreCfg == nil {
		return nil, false, nil
	}
	provider := strings.TrimSpace(coreCfg.TitleGenerationProvider())
	if provider == "" {
		return nil, false, nil
	}
	factory, ok := b.factories[provider]
	if !ok {
		return nil, false, fmt.Errorf("unsupported title generation provider: %s", provider)
	}
	return factory.Build(coreCfg)
}

type openRouterTitleGeneratorFactory struct{}

func (openRouterTitleGeneratorFactory) Build(coreCfg titleGenerationProviderConfigResolver) (titleGenerator, bool, error) {
	if coreCfg == nil {
		return nil, false, nil
	}
	apiKey := strings.TrimSpace(coreCfg.TitleGenerationOpenRouterAPIKey())
	apiKeyEnv := strings.TrimSpace(coreCfg.TitleGenerationOpenRouterAPIKeyEnv())
	if apiKey == "" && apiKeyEnv != "" {
		apiKey = strings.TrimSpace(os.Getenv(apiKeyEnv))
	}
	if apiKey == "" {
		return nil, false, nil
	}
	timeoutSeconds := coreCfg.TitleGenerationTimeoutSeconds()
	if timeoutSeconds <= 0 {
		timeoutSeconds = 10
	}
	provider := &openRouterTitleGenerator{
		baseURL: strings.TrimRight(strings.TrimSpace(coreCfg.TitleGenerationOpenRouterBaseURL()), "/"),
		apiKey:  apiKey,
		model:   strings.TrimSpace(coreCfg.TitleGenerationModel()),
		client: &http.Client{
			Timeout: time.Duration(timeoutSeconds) * time.Second,
		},
	}
	if provider.baseURL == "" {
		provider.baseURL = "https://openrouter.ai/api/v1"
	}
	if provider.model == "" {
		provider.model = "openrouter/auto"
	}
	return provider, true, nil
}

type openRouterTitleGenerator struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

type titleProviderError struct {
	Provider   string
	Kind       string
	StatusCode int
}

func (e *titleProviderError) Error() string {
	if e == nil {
		return "title provider error"
	}
	provider := strings.TrimSpace(e.Provider)
	if provider == "" {
		provider = "title provider"
	}
	switch strings.TrimSpace(e.Kind) {
	case "http_status":
		if e.StatusCode > 0 {
			return fmt.Sprintf("%s request failed with status %d", provider, e.StatusCode)
		}
		return provider + " request failed"
	case "invalid_response":
		return provider + " returned an invalid response payload"
	case "empty_title":
		return provider + " returned an empty title"
	default:
		return provider + " request failed"
	}
}

type openRouterChatRequest struct {
	Model       string                         `json:"model"`
	Messages    []openRouterChatRequestMessage `json:"messages"`
	Temperature float64                        `json:"temperature,omitempty"`
	MaxTokens   int                            `json:"max_tokens,omitempty"`
}

type openRouterChatRequestMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func (g *openRouterTitleGenerator) GenerateTitle(ctx context.Context, prompt string) (string, error) {
	if g == nil {
		return "", errors.New("title generator is nil")
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", errors.New("prompt is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if g.client == nil {
		g.client = &http.Client{Timeout: 10 * time.Second}
	}
	body := openRouterChatRequest{
		Model: g.model,
		Messages: []openRouterChatRequestMessage{
			{
				Role:    "system",
				Content: "Generate a concise, descriptive title for a coding conversation. Return title text only.",
			},
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Temperature: 0.2,
		MaxTokens:   32,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	url := strings.TrimRight(g.baseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+g.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	rawBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", &titleProviderError{
			Provider:   "openrouter",
			Kind:       "http_status",
			StatusCode: resp.StatusCode,
		}
	}
	var parsed map[string]any
	if err := json.Unmarshal(rawBody, &parsed); err != nil {
		return "", &titleProviderError{
			Provider: "openrouter",
			Kind:     "invalid_response",
		}
	}
	title := strings.TrimSpace(extractOpenRouterTitleFromPayload(parsed))
	if title == "" {
		return "", &titleProviderError{
			Provider: "openrouter",
			Kind:     "empty_title",
		}
	}
	return title, nil
}

func extractOpenRouterTitleFromPayload(parsed map[string]any) string {
	if len(parsed) == 0 {
		return ""
	}
	if title := strings.TrimSpace(extractOpenRouterContentValue(parsed["output_text"])); title != "" {
		return title
	}
	choicesRaw, _ := parsed["choices"].([]any)
	if len(choicesRaw) == 0 {
		return ""
	}
	firstChoice, _ := choicesRaw[0].(map[string]any)
	if len(firstChoice) == 0 {
		return ""
	}
	if text := strings.TrimSpace(asString(firstChoice["text"])); text != "" {
		return text
	}
	if text := strings.TrimSpace(extractOpenRouterContentValue(firstChoice["output_text"])); text != "" {
		return text
	}
	if message, ok := firstChoice["message"].(map[string]any); ok {
		if text := strings.TrimSpace(extractOpenRouterContentValue(message["content"])); text != "" {
			return text
		}
	}
	if delta, ok := firstChoice["delta"].(map[string]any); ok {
		if text := strings.TrimSpace(extractOpenRouterContentValue(delta["content"])); text != "" {
			return text
		}
	}
	return ""
}

func extractOpenRouterContentValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			segment := strings.TrimSpace(extractOpenRouterContentValue(item))
			if segment != "" {
				parts = append(parts, segment)
			}
		}
		return strings.TrimSpace(strings.Join(parts, " "))
	case map[string]any:
		if text := strings.TrimSpace(asString(typed["text"])); text != "" {
			return text
		}
		if inner := strings.TrimSpace(extractOpenRouterContentValue(typed["content"])); inner != "" {
			return inner
		}
		if inner := strings.TrimSpace(extractOpenRouterContentValue(typed["value"])); inner != "" {
			return inner
		}
	}
	return ""
}

func newTitleGeneratorFromCoreConfig(coreCfg config.CoreConfig, logger logging.Logger) (titleGenerator, error) {
	generator, err := buildTitleGeneratorFromCoreConfig(coreCfg, logger)
	if err != nil {
		if errors.Is(err, errTitleGeneratorNotConfigured) {
			return nil, nil
		}
		return nil, err
	}
	return generator, nil
}
