package daemon

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
)

type claudeProvider struct {
	cmdName string
}

type claudeRunner struct {
	cmdName string
	cwd     string
	env     []string
	sink    *logSink
	items   *itemSink

	mu        sync.Mutex
	sessionID string
	onSession func(string)
}

func newClaudeProvider(cmdName string) (Provider, error) {
	if strings.TrimSpace(cmdName) == "" {
		return nil, errors.New("command name is required")
	}
	return &claudeProvider{cmdName: cmdName}, nil
}

func (p *claudeProvider) Name() string {
	return "claude"
}

func (p *claudeProvider) Command() string {
	return p.cmdName
}

func (p *claudeProvider) Start(cfg StartSessionConfig, sink *logSink, items *itemSink) (*providerProcess, error) {
	if cfg.Resume {
		if strings.TrimSpace(cfg.ProviderSessionID) == "" {
			return nil, errors.New("provider session id is required to resume")
		}
	}

	runner := &claudeRunner{
		cmdName:   p.cmdName,
		cwd:       cfg.Cwd,
		env:       append([]string{}, cfg.Env...),
		sink:      sink,
		items:     items,
		sessionID: strings.TrimSpace(cfg.ProviderSessionID),
		onSession: cfg.OnProviderSessionID,
	}

	done := make(chan struct{})
	closeDone := sync.OnceFunc(func() { close(done) })

	if strings.TrimSpace(cfg.InitialText) != "" {
		if err := runner.SendUser(cfg.InitialText); err != nil {
			closeDone()
			return nil, err
		}
	}

	threadID := strings.TrimSpace(cfg.ProviderSessionID)
	return &providerProcess{
		Process: nil,
		Wait: func() error {
			<-done
			return nil
		},
		Interrupt: func() error {
			closeDone()
			return nil
		},
		ThreadID: threadID,
		Send:     runner.Send,
	}, nil
}

func (r *claudeRunner) Send(payload []byte) error {
	if r == nil {
		return errors.New("runner is nil")
	}
	if len(payload) == 0 {
		return errors.New("payload is required")
	}
	text, err := extractClaudeUserText(payload)
	if err != nil {
		return err
	}
	if strings.TrimSpace(text) == "" {
		return errors.New("text is required")
	}
	r.appendUserItem(text)
	return r.run(text)
}

func (r *claudeRunner) SendUser(text string) error {
	payload := buildClaudeUserPayload(text)
	return r.Send(payload)
}

func (r *claudeRunner) run(text string) error {
	args := []string{
		"--print",
		"--verbose",
		"--output-format", "stream-json",
	}
	if strings.TrimSpace(os.Getenv("ARCHON_CLAUDE_INCLUDE_PARTIAL")) == "1" {
		args = append(args, "--include-partial-messages")
	}
	sessionID := r.getSessionID()
	if sessionID != "" {
		args = append(args, "--resume", sessionID)
	}
	args = append(args, text)

	cmd := exec.Command(r.cmdName, args...)
	if r.cwd != "" {
		cmd.Dir = r.cwd
	}
	if len(r.env) > 0 {
		cmd.Env = append(os.Environ(), r.env...)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = readClaudeStream(stdoutPipe, r.sink, r.items, r.updateSessionID)
	}()
	go func() {
		defer wg.Done()
		_ = readClaudeStream(stderrPipe, nil, r.items, r.updateSessionID)
	}()

	err = cmd.Wait()
	wg.Wait()
	return err
}

func (r *claudeRunner) appendUserItem(text string) {
	if r == nil || r.items == nil {
		return
	}
	r.items.Append(map[string]any{
		"type": "userMessage",
		"content": []map[string]any{
			{"type": "text", "text": text},
		},
	})
}

func (r *claudeRunner) getSessionID() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sessionID
}

func (r *claudeRunner) updateSessionID(id string) {
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	r.mu.Lock()
	changed := r.sessionID != id
	r.sessionID = id
	r.mu.Unlock()
	if changed && r.onSession != nil {
		r.onSession(id)
	}
}

func readClaudeStream(r io.Reader, sink *logSink, items *itemSink, onSessionID func(string)) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	state := &ClaudeParseState{}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parsedItems, sessionID, err := ParseClaudeLine(line, state)
		if err != nil {
			if items != nil {
				items.Append(map[string]any{
					"type": "log",
					"text": line,
				})
			}
			if sink != nil {
				sink.Write("stderr", []byte("claude parse error: "+err.Error()+"\n"))
			}
			continue
		}
		if sessionID != "" && onSessionID != nil {
			onSessionID(sessionID)
		}
		if items != nil {
			for _, item := range parsedItems {
				if item == nil {
					continue
				}
				items.Append(item)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}

func buildClaudeUserPayload(text string) []byte {
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
	data, _ := json.Marshal(payload)
	return data
}

// session_id is provided by the CLI in the first system init event.

func extractClaudeUserText(payload []byte) (string, error) {
	var body map[string]any
	if err := json.Unmarshal(payload, &body); err != nil {
		return "", err
	}
	if typ, _ := body["type"].(string); typ != "user" {
		return "", errors.New("unsupported payload type")
	}
	text := extractClaudeMessageText(body["message"])
	return text, nil
}
