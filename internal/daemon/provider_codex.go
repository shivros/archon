package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"control/internal/types"
)

const (
	codexInitTimeout = 5 * time.Second
)

type codexProvider struct {
	cmdName string
	model   string
}

func newCodexProvider(cmdName string) (Provider, error) {
	if strings.TrimSpace(cmdName) == "" {
		return nil, errors.New("command name is required")
	}
	model := loadCoreConfigOrDefault().CodexDefaultModel()
	return &codexProvider{
		cmdName: cmdName,
		model:   model,
	}, nil
}

func (p *codexProvider) Name() string {
	return "codex"
}

func (p *codexProvider) Command() string {
	return fmt.Sprintf("%s app-server", p.cmdName)
}

func (p *codexProvider) Start(cfg StartSessionConfig, sink ProviderLogSink, items ProviderItemSink) (*providerProcess, error) {
	cmd := exec.Command(p.cmdName, "app-server")
	if cfg.Cwd != "" {
		cmd.Dir = cfg.Cwd
	}
	env := os.Environ()
	if strings.TrimSpace(cfg.CodexHome) != "" {
		env = append(env, "CODEX_HOME="+cfg.CodexHome)
	}
	if len(cfg.Env) > 0 {
		env = append(env, cfg.Env...)
	}
	if len(env) > 0 {
		cmd.Env = env
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	go func() {
		_, _ = io.Copy(sink.StderrWriter(), stderrPipe)
	}()

	controller := newCodexController(stdinPipe, stdoutPipe, sink)
	go controller.readLoop()

	ctx, cancel := context.WithTimeout(context.Background(), codexInitTimeout)
	defer cancel()

	if err := controller.initialize(ctx); err != nil {
		_ = cmd.Process.Kill()
		return nil, err
	}

	model := p.model
	if cfg.RuntimeOptions != nil {
		if override := strings.TrimSpace(cfg.RuntimeOptions.Model); override != "" {
			model = override
		}
	}
	threadID, err := controller.startThread(ctx, model, cfg.Cwd, cfg.RuntimeOptions)
	if err != nil {
		_ = cmd.Process.Kill()
		return nil, err
	}

	turnID, err := controller.startTurn(ctx, threadID, strings.Join(cfg.Args, " "), cfg.RuntimeOptions, model)
	if err != nil {
		_ = cmd.Process.Kill()
		return nil, err
	}

	controller.setTurn(threadID, turnID)
	controller.onTurnCompleted = func() {
		_ = signalTerminate(cmd.Process)
	}

	return &providerProcess{
		Process: cmd.Process,
		Wait:    cmd.Wait,
		Interrupt: func() error {
			return controller.interrupt(context.Background())
		},
		ThreadID: threadID,
	}, nil
}

type codexController struct {
	stdin  io.Writer
	reader *bufio.Scanner
	sink   ProviderLogSink

	mu              sync.Mutex
	nextID          int
	pending         map[int]chan codexMessage
	threadID        string
	turnID          string
	closed          bool
	onTurnCompleted func()
}

type codexMessage struct {
	ID     *int            `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *codexError     `json:"error,omitempty"`
}

type codexError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func newCodexController(stdin io.Writer, stdout io.Reader, sink ProviderLogSink) *codexController {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	return &codexController{
		stdin:   stdin,
		reader:  scanner,
		sink:    sink,
		pending: make(map[int]chan codexMessage),
	}
}

func (c *codexController) readLoop() {
	for c.reader.Scan() {
		line := strings.TrimSpace(c.reader.Text())
		if line == "" {
			continue
		}
		var msg codexMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			c.sink.Write("stderr", []byte("codex parse error: "+err.Error()+"\n"))
			continue
		}
		if msg.ID != nil {
			c.deliverResponse(msg)
			continue
		}
		c.handleNotification(msg)
	}
	c.closePending(errors.New("codex output closed"))
}

func (c *codexController) deliverResponse(msg codexMessage) {
	c.mu.Lock()
	ch := c.pending[*msg.ID]
	delete(c.pending, *msg.ID)
	c.mu.Unlock()
	if ch != nil {
		ch <- msg
		close(ch)
	}
}

func (c *codexController) closePending(err error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	for _, ch := range c.pending {
		ch <- codexMessage{Error: &codexError{Message: err.Error()}}
		close(ch)
	}
	c.pending = make(map[int]chan codexMessage)
	c.mu.Unlock()
}

func (c *codexController) handleNotification(msg codexMessage) {
	switch msg.Method {
	case "item/agentMessage/delta":
		if text := extractAgentDelta(msg.Params); text != "" {
			c.sink.Write("stdout", []byte(text))
		}
	case "item/commandExecution/outputDelta":
		stream, chunk := extractCommandOutputDelta(msg.Params)
		if chunk != "" {
			if stream == "" {
				stream = "stdout"
			}
			c.sink.Write(stream, []byte(chunk))
		}
	case "turn/completed":
		if c.onTurnCompleted != nil {
			c.onTurnCompleted()
		}
	}
}

func extractAgentDelta(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var payload struct {
		Delta   string `json:"delta"`
		Text    string `json:"text"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	if payload.Delta != "" {
		return payload.Delta
	}
	if payload.Text != "" {
		return payload.Text
	}
	return payload.Content
}

func extractCommandOutputDelta(raw json.RawMessage) (string, string) {
	if len(raw) == 0 {
		return "", ""
	}
	var payload struct {
		Stream string `json:"stream"`
		Chunk  string `json:"chunk"`
		Stdout string `json:"stdout"`
		Stderr string `json:"stderr"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", ""
	}
	if payload.Chunk != "" {
		return payload.Stream, payload.Chunk
	}
	if payload.Stdout != "" {
		return "stdout", payload.Stdout
	}
	if payload.Stderr != "" {
		return "stderr", payload.Stderr
	}
	return "", ""
}

func (c *codexController) initialize(ctx context.Context) error {
	_, err := c.request(ctx, "initialize", map[string]any{
		"clientInfo": map[string]string{
			"name":    "archon_cli",
			"title":   "Archon CLI",
			"version": "0.0.0",
		},
	})
	if err != nil {
		return err
	}
	return c.notify("initialized", map[string]any{})
}

func (c *codexController) startThread(ctx context.Context, model, cwd string, runtimeOptions *types.SessionRuntimeOptions) (string, error) {
	params := map[string]any{
		"model": model,
	}
	if cwd != "" {
		params["cwd"] = cwd
	}
	if opts := codexThreadOptions(runtimeOptions); len(opts) > 0 {
		for key, value := range opts {
			params[key] = value
		}
	}
	resp, err := c.request(ctx, "thread/start", params)
	if err != nil {
		return "", err
	}
	var result struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return "", err
	}
	if result.Thread.ID == "" {
		return "", errors.New("codex thread id missing")
	}
	return result.Thread.ID, nil
}

func (c *codexController) startTurn(ctx context.Context, threadID, inputText string, runtimeOptions *types.SessionRuntimeOptions, model string) (string, error) {
	if strings.TrimSpace(inputText) == "" {
		return "", errors.New("codex input is required")
	}
	input := []map[string]string{
		{
			"type": "text",
			"text": inputText,
		},
	}
	params := map[string]any{
		"threadId": threadID,
		"input":    input,
	}
	if strings.TrimSpace(model) != "" {
		params["model"] = strings.TrimSpace(model)
	}
	if opts := codexTurnOptions(runtimeOptions); len(opts) > 0 {
		for key, value := range opts {
			params[key] = value
		}
	}
	resp, err := c.request(ctx, "turn/start", params)
	if err != nil && strings.TrimSpace(model) != "" && shouldRetryWithoutModel(err) {
		delete(params, "model")
		resp, err = c.request(ctx, "turn/start", params)
	}
	if err != nil {
		return "", err
	}
	var result struct {
		Turn struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return "", err
	}
	return result.Turn.ID, nil
}

func (c *codexController) setTurn(threadID, turnID string) {
	c.mu.Lock()
	c.threadID = threadID
	c.turnID = turnID
	c.mu.Unlock()
}

func (c *codexController) interrupt(ctx context.Context) error {
	c.mu.Lock()
	threadID := c.threadID
	turnID := c.turnID
	c.mu.Unlock()
	if threadID == "" || turnID == "" {
		return errors.New("codex turn not initialized")
	}
	_, err := c.request(ctx, "turn/interrupt", map[string]any{
		"threadId": threadID,
		"turnId":   turnID,
	})
	return err
}

func (c *codexController) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	ch := make(chan codexMessage, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	if err := c.send(map[string]any{
		"method": method,
		"id":     id,
		"params": params,
	}); err != nil {
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case msg := <-ch:
		if msg.Error != nil {
			return nil, errors.New(msg.Error.Message)
		}
		return msg.Result, nil
	}
}

func (c *codexController) notify(method string, params any) error {
	return c.send(map[string]any{
		"method": method,
		"params": params,
	})
}

func (c *codexController) send(payload map[string]any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = c.stdin.Write(append(data, '\n'))
	return err
}
