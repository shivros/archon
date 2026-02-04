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
)

type codexAppServer struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	reader *bufio.Reader
	mu     sync.Mutex
	nextID int
	msgs   chan rpcMessage
	notes  chan rpcMessage
	errs   chan error
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

type codexThread struct {
	ID    string           `json:"id"`
	Turns []codexTurn      `json:"turns,omitempty"`
	Items []map[string]any `json:"items,omitempty"`
}

type codexTurn struct {
	ID    string           `json:"id"`
	Items []map[string]any `json:"items,omitempty"`
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

func startCodexAppServer(ctx context.Context, cwd, codexHome string) (*codexAppServer, error) {
	cmdName, err := findCommand("CONTROL_CODEX_CMD", "codex")
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

	client := &codexAppServer{
		cmd:    cmd,
		stdin:  stdin,
		reader: bufio.NewReader(stdout),
		nextID: 1,
		msgs:   make(chan rpcMessage, 32),
		notes:  make(chan rpcMessage, 64),
		errs:   make(chan error, 1),
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

func (c *codexAppServer) initialize(ctx context.Context) error {
	params := map[string]any{
		"clientInfo": map[string]any{
			"name":    "control",
			"title":   "Control",
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

func (c *codexAppServer) ResumeThread(ctx context.Context, threadID string) error {
	params := map[string]any{
		"threadId": threadID,
	}
	return c.request(ctx, "thread/resume", params, nil)
}

func (c *codexAppServer) StartTurn(ctx context.Context, threadID string, input []map[string]any) (string, error) {
	params := map[string]any{
		"threadId": threadID,
		"input":    input,
	}
	var result struct {
		Turn struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	if err := c.request(ctx, "turn/start", params, &result); err != nil {
		return "", err
	}
	if result.Turn.ID == "" {
		return "", errors.New("turn id missing")
	}
	return result.Turn.ID, nil
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
	if err := c.send(req); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-c.errs:
			if err != nil {
				return err
			}
		case msg := <-c.msgs:
			if msg.ID == nil || *msg.ID != id {
				continue
			}
			if msg.Error != nil {
				return fmt.Errorf("rpc error %d: %s", msg.Error.Code, msg.Error.Message)
			}
			if out != nil && len(msg.Result) > 0 {
				if err := json.Unmarshal(msg.Result, out); err != nil {
					return err
				}
			}
			return nil
		}
	}
}

func (c *codexAppServer) notify(method string, params any) error {
	payload := map[string]any{
		"method": method,
		"params": params,
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
			c.errs <- err
			return
		}
		var msg rpcMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}
		if msg.ID == nil {
			c.notes <- msg
		} else {
			c.msgs <- msg
		}
	}
}
