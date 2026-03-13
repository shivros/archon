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
	"time"

	"control/internal/types"
)

type claudeProvider struct {
	cmdName string
}

type claudeRunner struct {
	cmdName string
	cwd     string
	env     []string
	dirs    []string
	sink    ProviderSink
	items   ProviderItemSink
	options *types.SessionRuntimeOptions

	mu        sync.Mutex
	sessionID string
	onSession func(string)

	execMu          sync.Mutex
	nextExecID      int
	activeExecs     map[int]*claudeExecHandle
	interruptQueued bool
}

type claudeExecHandle struct {
	cmd         *exec.Cmd
	interrupted bool
}

var errClaudeTurnInterrupted = errors.New("claude turn interrupted")

var claudeRecursiveSessionEnv = []string{
	"CLAUDECODE",
	"CLAUDE_CODE_ENTRYPOINT",
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

func (p *claudeProvider) Start(cfg StartSessionConfig, sink ProviderSink, items ProviderItemSink) (*providerProcess, error) {
	if cfg.Resume {
		if strings.TrimSpace(cfg.ProviderSessionID) == "" {
			return nil, errors.New("provider session id is required to resume")
		}
	}

	runner := &claudeRunner{
		cmdName:   p.cmdName,
		cwd:       cfg.Cwd,
		env:       append([]string{}, cfg.Env...),
		dirs:      append([]string{}, cfg.AdditionalDirectories...),
		sink:      sink,
		items:     items,
		options:   types.CloneRuntimeOptions(cfg.RuntimeOptions),
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
			err := runner.Interrupt()
			closeDone()
			return err
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
	text, runtimeOptions, err := extractClaudeSendRequest(payload)
	if err != nil {
		return err
	}
	if strings.TrimSpace(text) == "" {
		return errors.New("text is required")
	}
	r.appendUserItem(text)
	return r.run(text, runtimeOptions)
}

func (r *claudeRunner) SendUser(text string) error {
	payload := buildClaudeUserPayloadWithRuntime(text, r.options)
	return r.Send(payload)
}

func (r *claudeRunner) run(text string, runtimeOptions *types.SessionRuntimeOptions) error {
	effectiveOptions := types.MergeRuntimeOptions(r.options, runtimeOptions)
	args := []string{
		"--print",
		"--verbose",
		"--output-format", "stream-json",
	}
	additionalDirArgs, err := providerAdditionalDirectoryArgs("claude", r.dirs)
	if err != nil {
		return err
	}
	args = append(args, additionalDirArgs...)
	if effectiveOptions != nil {
		if model := strings.TrimSpace(effectiveOptions.Model); model != "" {
			args = append(args, "--model", model)
		}
		if mode := claudeAccessToPermissionMode(effectiveOptions.Access); mode != "" {
			args = append(args, "--permission-mode", mode)
		}
	}
	if loadCoreConfigOrDefault().ClaudeIncludePartial() {
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
	cmd.Env = claudeCommandEnv(r.env)

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
	execID, shouldInterrupt := r.registerActiveExec(cmd)
	if shouldInterrupt {
		interruptClaudeProcess(cmd.Process)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = readClaudeStream(stdoutPipe, "provider_stdout_raw", r.sink, r.items, r.updateSessionID)
	}()
	go func() {
		defer wg.Done()
		_ = readClaudeStream(stderrPipe, "provider_stderr_raw", r.sink, r.items, r.updateSessionID)
	}()

	err = cmd.Wait()
	wg.Wait()
	if r.finishActiveExec(execID) {
		return errClaudeTurnInterrupted
	}
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

func (r *claudeRunner) Interrupt() error {
	if r == nil {
		return errors.New("runner is nil")
	}
	processes := make([]*os.Process, 0, 1)
	r.execMu.Lock()
	if len(r.activeExecs) == 0 {
		r.interruptQueued = true
		r.execMu.Unlock()
		return nil
	}
	for _, handle := range r.activeExecs {
		if handle == nil {
			continue
		}
		handle.interrupted = true
		if handle.cmd != nil && handle.cmd.Process != nil {
			processes = append(processes, handle.cmd.Process)
		}
	}
	r.execMu.Unlock()
	for _, process := range processes {
		interruptClaudeProcess(process)
	}
	return nil
}

func (r *claudeRunner) registerActiveExec(cmd *exec.Cmd) (int, bool) {
	r.execMu.Lock()
	defer r.execMu.Unlock()
	r.nextExecID++
	id := r.nextExecID
	handle := &claudeExecHandle{cmd: cmd}
	shouldInterrupt := r.interruptQueued
	if shouldInterrupt {
		handle.interrupted = true
		r.interruptQueued = false
	}
	if r.activeExecs == nil {
		r.activeExecs = make(map[int]*claudeExecHandle)
	}
	r.activeExecs[id] = handle
	return id, shouldInterrupt
}

func (r *claudeRunner) finishActiveExec(id int) bool {
	r.execMu.Lock()
	defer r.execMu.Unlock()
	handle := r.activeExecs[id]
	delete(r.activeExecs, id)
	if handle == nil {
		return false
	}
	return handle.interrupted
}

func interruptClaudeProcess(process *os.Process) {
	if process == nil {
		return
	}
	_ = signalTerminate(process)
	go func(p *os.Process) {
		time.Sleep(750 * time.Millisecond)
		_ = signalKill(p)
	}(process)
}

func readClaudeStream(r io.Reader, rawStream string, sink ProviderSink, items ProviderItemSink, onSessionID func(string)) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	state := &ClaudeParseState{}
	for scanner.Scan() {
		rawLine := scanner.Text()
		writeProviderDebug(sink, rawStream, []byte(rawLine+"\n"))
		line := strings.TrimSpace(rawLine)
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

func claudeCommandEnv(extra []string) []string {
	env := append([]string{}, os.Environ()...)
	env = append(env, extra...)
	for _, key := range claudeRecursiveSessionEnv {
		env = append(env, key+"=")
	}
	return env
}

func buildClaudeUserPayload(text string) []byte {
	return buildClaudeUserPayloadWithRuntime(text, nil)
}

func buildClaudeUserPayloadWithRuntime(text string, runtimeOptions *types.SessionRuntimeOptions) []byte {
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

// session_id is provided by the CLI in the first system init event.

func extractClaudeUserText(payload []byte) (string, error) {
	text, _, err := extractClaudeSendRequest(payload)
	return text, err
}

func extractClaudeSendRequest(payload []byte) (string, *types.SessionRuntimeOptions, error) {
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

func claudeAccessToPermissionMode(level types.AccessLevel) string {
	switch level {
	case types.AccessReadOnly:
		return "plan"
	case types.AccessOnRequest:
		return "default"
	case types.AccessFull:
		return "bypassPermissions"
	default:
		return ""
	}
}
