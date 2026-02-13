package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	"control/internal/config"
	"control/internal/types"
)

type openCodeProvider struct {
	providerName string
	client       *openCodeClient
}

type openCodeRunner struct {
	client     *openCodeClient
	sink       ProviderLogSink
	items      ProviderItemSink
	options    *types.SessionRuntimeOptions
	directory  string
	providerID string
	onSession  func(string)
}

func newOpenCodeProvider(providerName string) (Provider, error) {
	providerName = strings.ToLower(strings.TrimSpace(providerName))
	if providerName == "" {
		return nil, errors.New("provider name is required")
	}
	cfg := resolveOpenCodeClientConfig(providerName, loadCoreConfigOrDefault())
	client, err := newOpenCodeClient(cfg)
	if err != nil {
		return nil, err
	}
	return &openCodeProvider{
		providerName: providerName,
		client:       client,
	}, nil
}

func (p *openCodeProvider) Name() string {
	if p == nil {
		return ""
	}
	return p.providerName
}

func (p *openCodeProvider) Command() string {
	if p == nil || p.client == nil {
		return ""
	}
	return p.client.baseURL
}

func (p *openCodeProvider) Start(cfg StartSessionConfig, sink ProviderLogSink, items ProviderItemSink) (*providerProcess, error) {
	if p == nil || p.client == nil {
		return nil, errors.New("provider is not initialized")
	}
	providerSessionID := strings.TrimSpace(cfg.ProviderSessionID)
	if cfg.Resume && providerSessionID == "" {
		return nil, errors.New("provider session id is required to resume")
	}
	if providerSessionID == "" {
		createdID, err := p.createSession(context.Background(), cfg.Title, cfg.Cwd, sink)
		if err != nil {
			return nil, err
		}
		providerSessionID = createdID
	}

	runner := &openCodeRunner{
		client:     p.client,
		sink:       sink,
		items:      items,
		options:    types.CloneRuntimeOptions(cfg.RuntimeOptions),
		directory:  strings.TrimSpace(cfg.Cwd),
		providerID: providerSessionID,
		onSession:  cfg.OnProviderSessionID,
	}
	if runner.onSession != nil {
		runner.onSession(providerSessionID)
	}

	if text := strings.TrimSpace(cfg.InitialText); text != "" {
		if err := runner.SendUser(text); err != nil {
			return nil, err
		}
	}

	done := make(chan struct{})
	closeOnce := sync.OnceFunc(func() { close(done) })
	return &providerProcess{
		Process: nil,
		Wait: func() error {
			<-done
			return nil
		},
		Interrupt: func() error {
			err := runner.Interrupt()
			closeOnce()
			return err
		},
		ThreadID: "",
		Send:     runner.Send,
	}, nil
}

func (r *openCodeRunner) Send(payload []byte) error {
	if r == nil {
		return errors.New("runner is nil")
	}
	if len(payload) == 0 {
		return errors.New("payload is required")
	}
	text, runtimeOptions, err := extractOpenCodeSendRequest(payload)
	if err != nil {
		return err
	}
	if strings.TrimSpace(text) == "" {
		return errors.New("text is required")
	}
	return r.run(text, runtimeOptions)
}

func (r *openCodeRunner) SendUser(text string) error {
	return r.Send(buildOpenCodeUserPayloadWithRuntime(text, r.options))
}

func (r *openCodeRunner) run(text string, runtimeOptions *types.SessionRuntimeOptions) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("text is required")
	}
	effectiveOptions := types.MergeRuntimeOptions(r.options, runtimeOptions)
	r.appendUserItem(text)
	reply, err := r.client.Prompt(context.Background(), r.providerID, text, effectiveOptions, r.directory)
	if err != nil {
		if r.sink != nil {
			r.sink.Write("stderr", []byte("opencode prompt error: "+err.Error()+"\n"))
		}
		return err
	}
	reply = strings.TrimSpace(reply)
	if reply == "" {
		return nil
	}
	r.appendAssistantItem(reply)
	return nil
}

func (r *openCodeRunner) Interrupt() error {
	if r == nil || r.client == nil {
		return errors.New("runner is not initialized")
	}
	return r.client.AbortSession(context.Background(), r.providerID, r.directory)
}

func (r *openCodeRunner) appendUserItem(text string) {
	if r == nil || r.items == nil || strings.TrimSpace(text) == "" {
		return
	}
	r.items.Append(map[string]any{
		"type": "userMessage",
		"content": []map[string]any{
			{"type": "text", "text": text},
		},
	})
}

func (r *openCodeRunner) appendAssistantItem(text string) {
	if r == nil || r.items == nil || strings.TrimSpace(text) == "" {
		return
	}
	r.items.Append(map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": text},
			},
		},
	})
}

func (p *openCodeProvider) createSession(ctx context.Context, title, directory string, sink ProviderLogSink) (string, error) {
	if p == nil || p.client == nil {
		return "", errors.New("provider is not initialized")
	}
	sessionID, err := p.client.CreateSession(ctx, title, directory)
	if err == nil {
		return sessionID, nil
	}
	if !isOpenCodeUnreachable(err) {
		return "", err
	}
	startedBaseURL, startErr := maybeAutoStartOpenCodeServer(p.providerName, p.client.baseURL, p.client.token, sink)
	if startErr != nil {
		return "", errors.New(err.Error() + " (auto-start failed: " + startErr.Error() + ")")
	}
	if switchedClient, switchErr := cloneOpenCodeClientWithBaseURL(p.client, startedBaseURL); switchErr == nil {
		p.client = switchedClient
	}
	if sink != nil {
		sink.Write("stderr", []byte("opencode auto-start: retrying session create\n"))
	}
	deadline := time.Now().Add(12 * time.Second)
	retryDelay := 250 * time.Millisecond
	lastErr := err
	for time.Now().Before(deadline) {
		time.Sleep(retryDelay)
		sessionID, retryErr := p.client.CreateSession(ctx, title, directory)
		if retryErr == nil {
			return sessionID, nil
		}
		lastErr = retryErr
		if !isOpenCodeUnreachable(retryErr) {
			return "", retryErr
		}
		if retryDelay < 2*time.Second {
			retryDelay *= 2
		}
	}
	return "", lastErr
}

func buildOpenCodeUserPayloadWithRuntime(text string, runtimeOptions *types.SessionRuntimeOptions) []byte {
	text = strings.TrimSpace(text)
	payload := map[string]any{
		"type": "user",
		"message": map[string]any{
			"role": "user",
			"content": []map[string]any{
				{"type": "text", "text": text},
			},
		},
	}
	if runtimeOptions != nil {
		payload["runtime_options"] = runtimeOptions
	}
	data, _ := json.Marshal(payload)
	return data
}

func extractOpenCodeSendRequest(payload []byte) (string, *types.SessionRuntimeOptions, error) {
	var body map[string]any
	if err := json.Unmarshal(payload, &body); err != nil {
		return "", nil, err
	}
	if typ, _ := body["type"].(string); typ != "user" {
		return "", nil, errors.New("unsupported payload type")
	}
	text := extractClaudeMessageText(body["message"])
	var runtimeOptions *types.SessionRuntimeOptions
	if raw, ok := body["runtime_options"]; ok && raw != nil {
		data, err := json.Marshal(raw)
		if err == nil {
			var parsed types.SessionRuntimeOptions
			if err := json.Unmarshal(data, &parsed); err == nil {
				runtimeOptions = &parsed
			}
		}
	}
	return text, runtimeOptions, nil
}

func resolveOpenCodeClientConfig(provider string, coreCfg config.CoreConfig) openCodeClientConfig {
	provider = strings.ToLower(strings.TrimSpace(provider))
	baseURL := strings.TrimSpace(coreCfg.OpenCodeBaseURL(provider))
	baseEnv := openCodeBaseURLEnv(provider)
	if baseEnv != "" {
		if raw := strings.TrimSpace(os.Getenv(baseEnv)); raw != "" {
			baseURL = raw
		}
	}
	baseURL = resolveOpenCodeRuntimeBaseURL(provider, baseURL)
	token := strings.TrimSpace(coreCfg.OpenCodeToken(provider))
	for _, env := range openCodeTokenEnvs(provider, coreCfg.OpenCodeTokenEnv(provider)) {
		if raw := strings.TrimSpace(os.Getenv(env)); raw != "" {
			token = raw
			break
		}
	}
	return openCodeClientConfig{
		BaseURL:  baseURL,
		Username: coreCfg.OpenCodeUsername(provider),
		Token:    token,
		Timeout:  time.Duration(coreCfg.OpenCodeTimeoutSeconds(provider)) * time.Second,
	}
}

func openCodeBaseURLEnv(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "kilocode":
		return "KILOCODE_BASE_URL"
	default:
		return "OPENCODE_BASE_URL"
	}
}

func openCodeTokenEnvs(provider, configured string) []string {
	out := []string{}
	appendEnv := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		for _, existing := range out {
			if existing == value {
				return
			}
		}
		out = append(out, value)
	}
	appendEnv(configured)
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "kilocode":
		appendEnv("KILOCODE_TOKEN")
		appendEnv("KILOCODE_SERVER_PASSWORD")
	default:
		appendEnv("OPENCODE_TOKEN")
		appendEnv("OPENCODE_SERVER_PASSWORD")
	}
	return out
}
