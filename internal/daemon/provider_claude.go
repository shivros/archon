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

func newClaudeProvider() (Provider, error) {
	cmdName, err := findCommand("CONTROL_CLAUDE_CMD", "claude")
	if err != nil {
		return nil, err
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
	args := []string{
		"--print",
		"--verbose",
		"--output-format", "stream-json",
		"--input-format", "stream-json",
		"--replay-user-messages",
	}
	if strings.TrimSpace(os.Getenv("CONTROL_CLAUDE_INCLUDE_PARTIAL")) == "1" {
		args = append(args, "--include-partial-messages")
	}
	if cfg.Resume {
		sessionID := strings.TrimSpace(cfg.ProviderSessionID)
		if sessionID == "" {
			return nil, errors.New("provider session id is required to resume")
		}
		args = append(args, "--resume", sessionID)
	} else if strings.TrimSpace(cfg.ProviderSessionID) != "" {
		args = append(args, "--session-id", strings.TrimSpace(cfg.ProviderSessionID))
	}

	cmd := exec.Command(p.cmdName, args...)
	if cfg.Cwd != "" {
		cmd.Dir = cfg.Cwd
	}
	if len(cfg.Env) > 0 {
		cmd.Env = append(os.Environ(), cfg.Env...)
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

	writer := &claudeInputWriter{w: stdinPipe}

	go func() {
		_ = readClaudeStream(stdoutPipe, sink, items, cfg.OnProviderSessionID)
	}()
	go func() {
		_ = readClaudeStream(stderrPipe, nil, items, cfg.OnProviderSessionID)
	}()

	if strings.TrimSpace(cfg.InitialText) != "" {
		if err := writer.SendUser(cfg.InitialText); err != nil && sink != nil {
			sink.Write("stderr", []byte("claude send error: "+err.Error()+"\n"))
		}
	}

	threadID := strings.TrimSpace(cfg.ProviderSessionID)
	return &providerProcess{
		Process: cmd.Process,
		Wait:    cmd.Wait,
		Interrupt: func() error {
			return signalTerminate(cmd.Process)
		},
		ThreadID: threadID,
		Send:     writer.Send,
	}, nil
}

type claudeInputWriter struct {
	w  io.Writer
	mu sync.Mutex
}

func (w *claudeInputWriter) Send(payload []byte) error {
	if w == nil || w.w == nil {
		return errors.New("stdin is not available")
	}
	if len(payload) == 0 {
		return errors.New("payload is required")
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if !strings.HasSuffix(string(payload), "\n") {
		payload = append(payload, '\n')
	}
	_, err := w.w.Write(payload)
	return err
}

func (w *claudeInputWriter) SendUser(text string) error {
	payload := buildClaudeUserPayload(text)
	return w.Send(payload)
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
