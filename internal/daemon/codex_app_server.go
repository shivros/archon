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

	"control/internal/logging"
	"control/internal/providers"
	"control/internal/types"
)

type codexAppServer struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	reader *bufio.Reader
	mu     sync.Mutex
	nextID int
	msgs   chan rpcMessage
	notes  chan rpcMessage
	reqs   chan rpcMessage
	errs   chan error
	logger logging.Logger
	reqMu  sync.Mutex
	reqMap map[int]requestInfo
}

type rpcMessage struct {
	ID     *int            `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type requestInfo struct {
	method string
	start  time.Time
}

type codexThread struct {
	ID    string           `json:"id"`
	Turns []codexTurn      `json:"turns,omitempty"`
	Items []map[string]any `json:"items,omitempty"`
}

type codexTurn struct {
	ID     string           `json:"id"`
	Status string           `json:"status,omitempty"`
	Items  []map[string]any `json:"items,omitempty"`
}

type codexThreadListResult struct {
	Data       []codexThreadSummary `json:"data"`
	NextCursor *string              `json:"nextCursor"`
}

type codexThreadSummary struct {
	ID            string `json:"id"`
	Preview       string `json:"preview"`
	ModelProvider string `json:"modelProvider"`
	CreatedAt     int64  `json:"createdAt"`
	UpdatedAt     int64  `json:"updatedAt"`
	Cwd           string `json:"cwd,omitempty"`
}

type codexThreadReadResult struct {
	Thread *codexThread `json:"thread"`
}

type codexModelListResult struct {
	Data       []codexModelSummary `json:"data"`
	NextCursor *string             `json:"nextCursor"`
}

type codexModelSummary struct {
	ID                     string                    `json:"id"`
	Model                  string                    `json:"model"`
	DisplayName            string                    `json:"displayName"`
	Upgrade                string                    `json:"upgrade"`
	DefaultReasoningEffort string                    `json:"defaultReasoningEffort"`
	ReasoningEffort        []codexReasoningEffortDef `json:"reasoningEffort"`
	IsDefault              bool                      `json:"isDefault"`
}

type codexReasoningEffortDef struct {
	Effort      string `json:"effort"`
	Description string `json:"description"`
}

func startCodexAppServer(ctx context.Context, cwd, codexHome string, logger logging.Logger) (*codexAppServer, error) {
	if logger == nil {
		logger = logging.Nop()
	}
	def, _ := providers.Lookup("codex")
	cmdName, err := resolveProviderCommandName(def, "")
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(cmdName, "app-server")
	if cwd != "" {
		cmd.Dir = cwd
	}
	if strings.TrimSpace(codexHome) != "" {
		cmd.Env = append(os.Environ(), "CODEX_HOME="+codexHome)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}
	go io.Copy(io.Discard, stderr)

	logger.Info("codex_start", logging.F("cmd", cmdName), logging.F("cwd", cwd), logging.F("codex_home", codexHome))

	client := &codexAppServer{
		cmd:    cmd,
		stdin:  stdin,
		reader: bufio.NewReader(stdout),
		nextID: 1,
		msgs:   make(chan rpcMessage, 32),
		notes:  make(chan rpcMessage, 64),
		reqs:   make(chan rpcMessage, 16),
		errs:   make(chan error, 1),
		logger: logger,
		reqMap: make(map[int]requestInfo),
	}
	go client.readLoop()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.initialize(ctx); err != nil {
		client.Close()
		return nil, err
	}
	return client, nil
}

func (c *codexAppServer) Close() {
	if c == nil {
		return
	}
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	if c.stdin != nil {
		_ = c.stdin.Close()
	}
}

func (c *codexAppServer) Notifications() <-chan rpcMessage {
	if c == nil {
		return nil
	}
	return c.notes
}

func (c *codexAppServer) Errors() <-chan error {
	if c == nil {
		return nil
	}
	return c.errs
}

func (c *codexAppServer) Requests() <-chan rpcMessage {
	if c == nil {
		return nil
	}
	return c.reqs
}

func (c *codexAppServer) initialize(ctx context.Context) error {
	params := map[string]any{
		"clientInfo": map[string]any{
			"name":    "archon",
			"title":   "Archon",
			"version": "dev",
		},
	}
	if err := c.request(ctx, "initialize", params, nil); err != nil {
		return err
	}
	return c.notify("initialized", map[string]any{})
}

func (c *codexAppServer) ListThreads(ctx context.Context, cursor *string) (*codexThreadListResult, error) {
	params := map[string]any{}
	if cursor != nil {
		params["cursor"] = *cursor
	}
	var result codexThreadListResult
	if err := c.request(ctx, "thread/list", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *codexAppServer) ReadThread(ctx context.Context, threadID string) (*codexThread, error) {
	params := map[string]any{
		"threadId":     threadID,
		"includeTurns": true,
	}
	var result codexThreadReadResult
	if err := c.request(ctx, "thread/read", params, &result); err != nil {
		return nil, err
	}
	if result.Thread == nil {
		return nil, errors.New("thread not found")
	}
	return result.Thread, nil
}

func (c *codexAppServer) ListModels(ctx context.Context, cursor *string, limit int) (*codexModelListResult, error) {
	params := map[string]any{}
	if cursor != nil && strings.TrimSpace(*cursor) != "" {
		params["cursor"] = strings.TrimSpace(*cursor)
	}
	if limit > 0 {
		params["limit"] = limit
	}
	var result codexModelListResult
	if err := c.request(ctx, "model/list", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *codexAppServer) ResumeThread(ctx context.Context, threadID string) error {
	params := map[string]any{
		"threadId": threadID,
	}
	err := c.request(ctx, "thread/resume", params, nil)
	if err != nil {
		if c.logger != nil {
			c.logger.Warn("codex_thread_resume_error",
				logging.F("thread_id", threadID),
				logging.F("error", err.Error()),
			)
		}
		return err
	}
	if c.logger != nil {
		c.logger.Info("codex_thread_resume_ok", logging.F("thread_id", threadID))
	}
	return nil
}

func (c *codexAppServer) StartThread(ctx context.Context, model, cwd string, runtimeOptions *types.SessionRuntimeOptions) (string, error) {
	params := map[string]any{}
	if strings.TrimSpace(model) != "" {
		params["model"] = strings.TrimSpace(model)
	}
	if strings.TrimSpace(cwd) != "" {
		params["cwd"] = strings.TrimSpace(cwd)
	}
	if opts := codexThreadOptions(runtimeOptions); len(opts) > 0 {
		for key, value := range opts {
			params[key] = value
		}
	}
	if c.logger != nil {
		c.logger.Info("codex_thread_start_request",
			logging.F("model", model),
			logging.F("cwd", cwd),
		)
	}
	var result struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if err := c.request(ctx, "thread/start", params, &result); err != nil {
		if c.logger != nil {
			c.logger.Error("codex_thread_start_error",
				logging.F("model", model),
				logging.F("cwd", cwd),
				logging.F("error", err.Error()),
			)
		}
		return "", err
	}
	if strings.TrimSpace(result.Thread.ID) == "" {
		return "", errors.New("codex thread id missing")
	}
	if c.logger != nil {
		c.logger.Info("codex_thread_start_ok",
			logging.F("model", model),
			logging.F("thread_id", result.Thread.ID),
		)
	}
	return strings.TrimSpace(result.Thread.ID), nil
}

func (c *codexAppServer) StartTurn(ctx context.Context, threadID string, input []map[string]any, runtimeOptions *types.SessionRuntimeOptions, model string) (string, error) {
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
	var result struct {
		Turn struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	if err := c.request(ctx, "turn/start", params, &result); err != nil {
		if strings.TrimSpace(model) != "" && shouldRetryWithoutModel(err) {
			delete(params, "model")
			if retryErr := c.request(ctx, "turn/start", params, &result); retryErr != nil {
				return "", retryErr
			}
			if result.Turn.ID == "" {
				return "", errors.New("turn id missing")
			}
			return result.Turn.ID, nil
		}
		return "", err
	}
	if result.Turn.ID == "" {
		return "", errors.New("turn id missing")
	}
	return result.Turn.ID, nil
}

func (c *codexAppServer) InterruptTurn(ctx context.Context, threadID, turnID string) error {
	params := map[string]any{
		"threadId": threadID,
		"turnId":   turnID,
	}
	return c.request(ctx, "turn/interrupt", params, nil)
}

func (c *codexAppServer) WaitForTurnCompleted(ctx context.Context, turnID string) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-c.errs:
			if err != nil {
				return err
			}
		case msg := <-c.notes:
			if msg.Method != "turn/completed" {
				continue
			}
			if len(msg.Params) == 0 {
				return nil
			}
			var payload struct {
				Turn struct {
					ID string `json:"id"`
				} `json:"turn"`
			}
			if err := json.Unmarshal(msg.Params, &payload); err != nil {
				return nil
			}
			if turnID == "" || payload.Turn.ID == turnID {
				return nil
			}
		}
	}
}

func (c *codexAppServer) request(ctx context.Context, method string, params any, out any) error {
	id := c.nextRequestID()
	req := map[string]any{
		"method": method,
		"id":     id,
		"params": params,
	}
	c.trackRequest(id, method)
	if c.logger != nil && c.logger.Enabled(logging.Debug) {
		c.logger.Debug("codex_send",
			logging.F("request_id", id),
			logging.F("method", method),
			logging.F("params_bytes", paramsSize(params)),
		)
	}
	if err := c.send(req); err != nil {
		if c.logger != nil {
			c.logger.Error("codex_send_error", logging.F("request_id", id), logging.F("method", method), logging.F("error", err))
		}
		return err
	}
	for {
		select {
		case <-ctx.Done():
			if c.logger != nil {
				c.logger.Warn("codex_timeout", logging.F("request_id", id), logging.F("method", method))
			}
			return ctx.Err()
		case err := <-c.errs:
			if err != nil {
				if c.logger != nil {
					c.logger.Error("codex_error", logging.F("error", err))
				}
				return err
			}
		case msg := <-c.msgs:
			if msg.ID == nil || *msg.ID != id {
				continue
			}
			c.finishRequest(id, msg.Error)
			if msg.Error != nil {
				if c.logger != nil {
					c.logger.Warn("codex_rpc_error",
						logging.F("request_id", id),
						logging.F("method", method),
						logging.F("code", msg.Error.Code),
						logging.F("message", msg.Error.Message),
					)
				}
				return fmt.Errorf("rpc error %d: %s", msg.Error.Code, msg.Error.Message)
			}
			if out != nil && len(msg.Result) > 0 {
				if err := json.Unmarshal(msg.Result, out); err != nil {
					if c.logger != nil {
						c.logger.Error("codex_unmarshal_error",
							logging.F("request_id", id),
							logging.F("method", method),
							logging.F("error", err),
						)
					}
					return err
				}
			}
			return nil
		}
	}
}

func (c *codexAppServer) notify(method string, params any) error {
	if c.logger != nil && c.logger.Enabled(logging.Debug) {
		c.logger.Debug("codex_notify",
			logging.F("method", method),
			logging.F("params_bytes", paramsSize(params)),
		)
	}
	payload := map[string]any{
		"method": method,
		"params": params,
	}
	return c.send(payload)
}

func (c *codexAppServer) respond(id int, result any) error {
	if c.logger != nil {
		c.logger.Info("codex_respond",
			logging.F("request_id", id),
			logging.F("result_bytes", paramsSize(result)),
		)
	}
	payload := map[string]any{
		"id":     id,
		"result": result,
	}
	return c.send(payload)
}

func (c *codexAppServer) respondError(id int, code int, message string) error {
	if c.logger != nil {
		c.logger.Warn("codex_respond_error",
			logging.F("request_id", id),
			logging.F("code", code),
			logging.F("message", message),
		)
	}
	payload := map[string]any{
		"id": id,
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	}
	return c.send(payload)
}

func (c *codexAppServer) send(payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = c.stdin.Write(append(data, '\n'))
	return err
}

func (c *codexAppServer) nextRequestID() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	id := c.nextID
	c.nextID++
	return id
}

func (c *codexAppServer) readLoop() {
	for {
		line, err := c.reader.ReadBytes('\n')
		if err != nil {
			if c.logger != nil {
				c.logger.Error("codex_read_error", logging.F("error", err))
			}
			c.errs <- err
			return
		}
		var msg rpcMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			if c.logger != nil {
				c.logger.Warn("codex_parse_error", logging.F("error", err))
			}
			continue
		}
		if msg.ID == nil {
			if c.logger != nil && c.logger.Enabled(logging.Debug) {
				c.logger.Debug("codex_event", logging.F("method", msg.Method), logging.F("params_bytes", len(msg.Params)))
			}
			c.notes <- msg
		} else if msg.Method != "" {
			if c.logger != nil && c.logger.Enabled(logging.Debug) {
				c.logger.Debug("codex_request", logging.F("request_id", *msg.ID), logging.F("method", msg.Method), logging.F("params_bytes", len(msg.Params)))
			}
			c.reqs <- msg
		} else {
			c.msgs <- msg
		}
	}
}

func (c *codexAppServer) trackRequest(id int, method string) {
	c.reqMu.Lock()
	defer c.reqMu.Unlock()
	c.reqMap[id] = requestInfo{method: method, start: time.Now()}
}

func (c *codexAppServer) finishRequest(id int, rpcErr *rpcError) {
	c.reqMu.Lock()
	info, ok := c.reqMap[id]
	if ok {
		delete(c.reqMap, id)
	}
	c.reqMu.Unlock()
	if !ok || c.logger == nil || !c.logger.Enabled(logging.Debug) {
		return
	}
	fields := []logging.Field{
		logging.F("request_id", id),
		logging.F("method", info.method),
		logging.F("latency_ms", time.Since(info.start).Milliseconds()),
	}
	if rpcErr != nil {
		fields = append(fields, logging.F("rpc_error", rpcErr.Message))
	}
	c.logger.Debug("codex_response", fields...)
}

func paramsSize(params any) int {
	if params == nil {
		return 0
	}
	data, err := json.Marshal(params)
	if err != nil {
		return 0
	}
	return len(data)
}
