package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"control/internal/daemon/acp"
	"control/internal/logging"
	"control/internal/types"
)

const hermesInitializeTimeout = 30 * time.Second

var errHermesSessionEnded = errors.New("hermes session ended")

type hermesProvider struct {
	cmdName      string
	defaultModel string
	extraArgs    []string
	extraEnv     []string
}

type hermesRuntimeRegistry struct {
	mu       sync.Mutex
	sessions map[string]*hermesRuntime
}

type hermesApprovalRequest struct {
	requestID int
	params    acp.RequestPermissionParams
	response  chan acp.RequestPermissionOutcome
}

type hermesPromptState struct {
	turnID string
	done   chan error
}

type hermesRuntime struct {
	sessionID string
	cwd       string
	sink      ProviderSink

	client   *acp.Client
	process  *os.Process
	threadID string

	hub *codexSubscriberHub

	approvalStore ApprovalStorage

	mu              sync.Mutex
	activeTurn      string
	pendingPrompt   *hermesPromptState
	pendingApproval map[int]*hermesApprovalRequest
	closed          bool

	nextApprovalID atomic.Int64
	waitOnce       sync.Once
	waitErr        error
	waitDone       chan struct{}
}

type hermesLiveSession struct {
	mu            sync.Mutex
	sessionID     string
	session       *types.Session
	meta          *types.SessionMeta
	manager       *SessionManager
	approvalStore ApprovalStorage
	logger        logging.Logger
	closed        bool
}

type hermesLiveSessionFactory struct {
	manager       *SessionManager
	approvalStore ApprovalStorage
	logger        logging.Logger
}

var sharedHermesRuntimes = &hermesRuntimeRegistry{
	sessions: map[string]*hermesRuntime{},
}

var (
	_ TurnCapableSession     = (*hermesLiveSession)(nil)
	_ ApprovalCapableSession = (*hermesLiveSession)(nil)
	_ closeAwareSession      = (*hermesLiveSession)(nil)
)

func newHermesProvider(cmdName string) (Provider, error) {
	if strings.TrimSpace(cmdName) == "" {
		return nil, errors.New("command name is required")
	}
	coreCfg := loadCoreConfigOrDefault()
	return &hermesProvider{
		cmdName:      cmdName,
		defaultModel: coreCfg.HermesDefaultModel(),
		extraArgs:    coreCfg.HermesArgs(),
		extraEnv:     coreCfg.HermesEnv(),
	}, nil
}

func (p *hermesProvider) Name() string {
	return "hermes"
}

func (p *hermesProvider) Command() string {
	parts := []string{p.cmdName}
	parts = append(parts, p.extraArgs...)
	parts = append(parts, "acp")
	return strings.TrimSpace(strings.Join(parts, " "))
}

func (p *hermesProvider) Start(cfg StartSessionConfig, sink ProviderSink, _ ProviderItemSink) (*providerProcess, error) {
	sessionID := strings.TrimSpace(cfg.SessionID)
	if sessionID == "" {
		return nil, errors.New("session id is required")
	}
	if runtime := sharedHermesRuntimes.Get(sessionID); runtime != nil && !runtime.IsClosed() {
		return runtime.providerProcess(), nil
	}

	args := append([]string{}, p.extraArgs...)
	args = append(args, "acp")

	env := os.Environ()
	env = append(env, p.extraEnv...)
	env = append(env, cfg.Env...)

	client, err := acp.Start(context.Background(), acp.StartOptions{
		Command:           p.cmdName,
		Args:              args,
		Env:               env,
		Cwd:               cfg.Cwd,
		Stderr:            sink.StderrWriter(),
		InitializeTimeout: hermesInitializeTimeout,
		ClientInfo: acp.ImplementationInfo{
			Name:    "archon",
			Version: "0.1.0",
		},
		ClientCapabilities: acp.ClientCapabilities{
			FS:       acp.FSCapabilities{},
			Terminal: false,
		},
		ProtocolVersion: acp.ProtocolVersion1,
		Logger: func(format string, args ...any) {
			if sink == nil {
				return
			}
			sink.WriteDebug("provider_debug", []byte(fmt.Sprintf(format, args...)+"\n"))
		},
	})
	if err != nil {
		return nil, err
	}

	runtime := &hermesRuntime{
		sessionID:       sessionID,
		cwd:             strings.TrimSpace(cfg.Cwd),
		sink:            sink,
		client:          client,
		process:         client.Process(),
		hub:             newCodexSubscriberHub(),
		pendingApproval: map[int]*hermesApprovalRequest{},
		approvalStore:   NopApprovalStorage{},
		waitDone:        make(chan struct{}),
	}
	runtime.client.RegisterHandler(acp.MethodRequestPermission, acp.HandlePermission(runtime.handlePermissionRequest))

	if cfg.Resume {
		if strings.TrimSpace(cfg.ProviderSessionID) == "" {
			_ = runtime.Close(context.Background())
			return nil, errors.New("provider session id is required to resume")
		}
		if !runtime.client.AgentCapabilities().LoadSession {
			_ = runtime.Close(context.Background())
			return nil, fmt.Errorf("%w: loadSession not supported", errHermesSessionEnded)
		}
		var loadResp acp.LoadSessionResult
		if err := runtime.client.Call(context.Background(), acp.MethodSessionLoad, acp.LoadSessionParams{
			SessionID:  strings.TrimSpace(cfg.ProviderSessionID),
			Cwd:        strings.TrimSpace(cfg.Cwd),
			McpServers: []acp.McpServer{},
		}, &loadResp); err != nil {
			_ = runtime.Close(context.Background())
			return nil, fmt.Errorf("%w: %v", errHermesSessionEnded, err)
		}
		runtime.threadID = strings.TrimSpace(loadResp.SessionID)
	} else {
		var newResp acp.NewSessionResult
		if err := runtime.client.Call(context.Background(), acp.MethodSessionNew, acp.NewSessionParams{
			Cwd:        strings.TrimSpace(cfg.Cwd),
			McpServers: []acp.McpServer{},
		}, &newResp); err != nil {
			_ = runtime.Close(context.Background())
			return nil, err
		}
		runtime.threadID = strings.TrimSpace(newResp.SessionID)
	}
	if runtime.threadID == "" {
		_ = runtime.Close(context.Background())
		return nil, errors.New("hermes session id missing")
	}
	if cfg.OnProviderSessionID != nil {
		cfg.OnProviderSessionID(runtime.threadID)
	}

	sharedHermesRuntimes.Set(sessionID, runtime)
	go runtime.notificationLoop(runtime.client.Subscribe())
	go runtime.waitLoop()
	return runtime.providerProcess(), nil
}

func (r *hermesRuntime) providerProcess() *providerProcess {
	if r == nil {
		return nil
	}
	return &providerProcess{
		Process:   r.process,
		Wait:      r.Wait,
		Interrupt: func() error { return r.Interrupt(context.Background()) },
		ThreadID:  r.threadID,
		Send:      r.Send,
	}
}

func (r *hermesRuntime) notificationLoop(sub <-chan acp.Notification) {
	for note := range sub {
		switch note.Method {
		case acp.MethodSessionUpdate:
			r.broadcast(note.Method, nil, note.Params)
		}
	}
}

func (r *hermesRuntime) waitLoop() {
	err := r.client.Wait()
	r.finishWait(err)
}

func (r *hermesRuntime) finishWait(err error) {
	r.waitOnce.Do(func() {
		r.mu.Lock()
		r.closed = true
		for id, pending := range r.pendingApproval {
			delete(r.pendingApproval, id)
			pending.response <- acp.CancelledOutcome()
			close(pending.response)
		}
		if prompt := r.pendingPrompt; prompt != nil {
			r.pendingPrompt = nil
			r.activeTurn = ""
			prompt.done <- errHermesSessionEnded
			close(prompt.done)
		}
		r.mu.Unlock()
		r.waitErr = err
		sharedHermesRuntimes.Delete(r.sessionID, r)
		close(r.waitDone)
	})
}

func (r *hermesRuntime) Wait() error {
	if r == nil {
		return nil
	}
	<-r.waitDone
	return r.waitErr
}

func (r *hermesRuntime) IsClosed() bool {
	if r == nil {
		return true
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.closed
}

func (r *hermesRuntime) Close(ctx context.Context) error {
	if r == nil || r.client == nil {
		return nil
	}
	err := r.client.Close(ctx)
	r.finishWait(err)
	return err
}

func (r *hermesRuntime) Send(payload []byte) error {
	text, _, err := extractOpenCodeSendRequest(payload)
	if err != nil {
		return err
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("text is required")
	}
	_, err = r.StartTurn(context.Background(), defaultTurnIDGenerator{}.NewTurnID("hermes"), text)
	return err
}

func (r *hermesRuntime) StartTurn(ctx context.Context, turnID, text string) (string, error) {
	if r == nil {
		return "", errors.New("hermes runtime is not initialized")
	}
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		turnID = defaultTurnIDGenerator{}.NewTurnID("hermes")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", errors.New("text is required")
	}

	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return "", errHermesSessionEnded
	}
	if r.pendingPrompt != nil {
		r.mu.Unlock()
		return "", errors.New("hermes turn already in progress")
	}
	prompt := &hermesPromptState{
		turnID: turnID,
		done:   make(chan error, 1),
	}
	r.pendingPrompt = prompt
	r.activeTurn = turnID
	r.mu.Unlock()

	r.broadcast("turn/started", nil, mustMarshalJSON(map[string]any{
		"turnId": turnID,
	}))

	go func() {
		var result acp.PromptResult
		err := r.client.Call(ctx, acp.MethodSessionPrompt, acp.PromptParams{
			SessionID: r.threadID,
			Prompt: []acp.ContentBlock{
				{Type: "text", Text: text},
			},
		}, &result)

		if err != nil {
			r.broadcast("turn/failed", nil, mustMarshalJSON(map[string]any{
				"turnId": turnID,
				"error":  strings.TrimSpace(err.Error()),
			}))
			r.completePrompt(prompt, err)
			return
		}

		method := "turn/completed"
		if strings.EqualFold(strings.TrimSpace(result.StopReason), acp.StopReasonCancelled) {
			method = "turn/failed"
		}
		payload := map[string]any{
			"turnId":     turnID,
			"stopReason": strings.TrimSpace(result.StopReason),
		}
		if method == "turn/failed" {
			payload["error"] = "turn cancelled"
		}
		r.broadcast(method, nil, mustMarshalJSON(payload))
		r.completePrompt(prompt, nil)
	}()

	return turnID, nil
}

func (r *hermesRuntime) completePrompt(prompt *hermesPromptState, err error) {
	r.mu.Lock()
	if r.pendingPrompt == prompt {
		r.pendingPrompt = nil
		r.activeTurn = ""
	}
	r.mu.Unlock()
	prompt.done <- err
	close(prompt.done)
}

func (r *hermesRuntime) Interrupt(ctx context.Context) error {
	if r == nil {
		return errors.New("hermes runtime is not initialized")
	}
	r.mu.Lock()
	prompt := r.pendingPrompt
	pendingApprovals := make([]*hermesApprovalRequest, 0, len(r.pendingApproval))
	for _, req := range r.pendingApproval {
		pendingApprovals = append(pendingApprovals, req)
	}
	r.mu.Unlock()
	if prompt == nil {
		return nil
	}
	for _, pending := range pendingApprovals {
		if pending == nil {
			continue
		}
		select {
		case pending.response <- acp.CancelledOutcome():
		default:
		}
	}
	if err := r.client.Notify(acp.MethodSessionCancel, acp.CancelParams{SessionID: r.threadID}); err != nil {
		return err
	}
	select {
	case err := <-prompt.done:
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, errHermesSessionEnded) {
			return err
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *hermesRuntime) handlePermissionRequest(ctx context.Context, params acp.RequestPermissionParams) (acp.RequestPermissionOutcome, error) {
	requestID := int(r.nextApprovalID.Add(1))
	req := &hermesApprovalRequest{
		requestID: requestID,
		params:    params,
		response:  make(chan acp.RequestPermissionOutcome, 1),
	}

	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return acp.CancelledOutcome(), nil
	}
	r.pendingApproval[requestID] = req
	store := r.approvalStore
	r.mu.Unlock()

	if store != nil {
		_ = store.StoreApproval(context.Background(), r.sessionID, requestID, acp.MethodRequestPermission, mustMarshalJSON(params))
	}
	r.broadcast(acp.MethodRequestPermission, &requestID, mustMarshalJSON(params))

	select {
	case outcome := <-req.response:
		r.broadcast("permission/replied", &requestID, mustMarshalJSON(map[string]any{
			"outcome": outcome,
		}))
		if store != nil {
			_ = store.DeleteApproval(context.Background(), r.sessionID, requestID)
		}
		r.mu.Lock()
		delete(r.pendingApproval, requestID)
		r.mu.Unlock()
		return outcome, nil
	case <-ctx.Done():
		r.mu.Lock()
		delete(r.pendingApproval, requestID)
		r.mu.Unlock()
		if store != nil {
			_ = store.DeleteApproval(context.Background(), r.sessionID, requestID)
		}
		return acp.CancelledOutcome(), nil
	}
}

func (r *hermesRuntime) Respond(_ context.Context, requestID int, result map[string]any) error {
	r.mu.Lock()
	req := r.pendingApproval[requestID]
	r.mu.Unlock()
	if req == nil {
		return notFoundError("approval not found", nil)
	}
	outcome := hermesApprovalOutcome(req.params.Options, result)
	select {
	case req.response <- outcome:
		return nil
	default:
		return errors.New("approval response already submitted")
	}
}

func (r *hermesRuntime) SetApprovalStorage(store ApprovalStorage) {
	if r == nil {
		return
	}
	if store == nil {
		store = NopApprovalStorage{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.approvalStore = store
}

func (r *hermesRuntime) ActiveTurnID() string {
	if r == nil {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.activeTurn
}

func (r *hermesRuntime) Events() (<-chan types.CodexEvent, func()) {
	if r == nil || r.hub == nil {
		ch := make(chan types.CodexEvent)
		close(ch)
		return ch, func() {}
	}
	return r.hub.Add()
}

func (r *hermesRuntime) broadcast(method string, id *int, params json.RawMessage) {
	if r == nil || r.hub == nil {
		return
	}
	r.hub.Broadcast(types.CodexEvent{
		ID:     id,
		Method: method,
		Params: params,
		TS:     time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func (r *hermesRuntimeRegistry) Get(sessionID string) *hermesRuntime {
	if r == nil {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sessions[sessionID]
}

func (r *hermesRuntimeRegistry) Set(sessionID string, runtime *hermesRuntime) {
	if r == nil || runtime == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	r.mu.Lock()
	r.sessions[sessionID] = runtime
	r.mu.Unlock()
}

func (r *hermesRuntimeRegistry) Delete(sessionID string, runtime *hermesRuntime) {
	if r == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing := r.sessions[sessionID]; existing == runtime {
		delete(r.sessions, sessionID)
	}
}

func newHermesLiveSessionFactory(manager *SessionManager, approvalStore ApprovalStorage, logger logging.Logger) *hermesLiveSessionFactory {
	if logger == nil {
		logger = logging.Nop()
	}
	if approvalStore == nil {
		approvalStore = NopApprovalStorage{}
	}
	return &hermesLiveSessionFactory{
		manager:       manager,
		approvalStore: approvalStore,
		logger:        logger,
	}
}

func (f *hermesLiveSessionFactory) ProviderName() string {
	return "hermes"
}

func (f *hermesLiveSessionFactory) CreateTurnCapable(_ context.Context, session *types.Session, meta *types.SessionMeta) (TurnCapableSession, error) {
	if session == nil {
		return nil, invalidError("session is required", nil)
	}
	ls := &hermesLiveSession{
		sessionID:     session.ID,
		session:       cloneSessionShallow(session),
		meta:          cloneSessionMeta(meta),
		manager:       f.manager,
		approvalStore: f.approvalStore,
		logger:        f.logger,
	}
	if runtime := ls.runtime(); runtime != nil {
		runtime.SetApprovalStorage(f.approvalStore)
	}
	return ls, nil
}

func (s *hermesLiveSession) SessionID() string {
	return s.sessionID
}

func (s *hermesLiveSession) ActiveTurnID() string {
	if runtime := s.runtime(); runtime != nil {
		return runtime.ActiveTurnID()
	}
	return ""
}

func (s *hermesLiveSession) SetSessionMeta(meta *types.SessionMeta) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.meta = cloneSessionMeta(meta)
}

func (s *hermesLiveSession) StartTurn(ctx context.Context, input []map[string]any, _ *types.SessionRuntimeOptions) (string, error) {
	text := extractTextInput(input)
	if text == "" {
		return "", invalidError("text input is required", nil)
	}
	runtime := s.runtime()
	if runtime == nil {
		return "", unavailableError("hermes session ended", errHermesSessionEnded)
	}
	turnID := defaultTurnIDGenerator{}.NewTurnID("hermes")
	return runtime.StartTurn(ctx, turnID, text)
}

func (s *hermesLiveSession) Interrupt(ctx context.Context) error {
	runtime := s.runtime()
	if runtime == nil {
		return nil
	}
	return runtime.Interrupt(ctx)
}

func (s *hermesLiveSession) Respond(ctx context.Context, requestID int, result map[string]any) error {
	runtime := s.runtime()
	if runtime == nil {
		return unavailableError("hermes session ended", errHermesSessionEnded)
	}
	return runtime.Respond(ctx, requestID, result)
}

func (s *hermesLiveSession) Events() (<-chan types.CodexEvent, func()) {
	runtime := s.runtime()
	if runtime == nil {
		ch := make(chan types.CodexEvent)
		close(ch)
		return ch, func() {}
	}
	return runtime.Events()
}

func (s *hermesLiveSession) Close() {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
}

func (s *hermesLiveSession) IsClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return true
	}
	runtime := s.runtime()
	return runtime == nil || runtime.IsClosed()
}

func (s *hermesLiveSession) runtime() *hermesRuntime {
	if s == nil {
		return nil
	}
	runtime := sharedHermesRuntimes.Get(s.sessionID)
	if runtime != nil {
		runtime.SetApprovalStorage(s.approvalStore)
	}
	return runtime
}

func hermesApprovalOutcome(options []acp.PermissionOption, result map[string]any) acp.RequestPermissionOutcome {
	decision := strings.ToLower(strings.TrimSpace(asString(result["decision"])))
	if decision == "" {
		decision = "accept"
	}
	if decision == "cancel" || decision == "cancelled" {
		return acp.CancelledOutcome()
	}

	preferAlways := false
	if acceptSettings, ok := result["acceptSettings"].(map[string]any); ok {
		if truthyApprovalFlag(acceptSettings["always"]) || truthyApprovalFlag(acceptSettings["remember"]) {
			preferAlways = true
		}
		if optionID := strings.TrimSpace(asString(acceptSettings["option_id"])); optionID != "" {
			return acp.Selected(optionID)
		}
	}

	target := "allow_once"
	switch decision {
	case "allow_always", "always":
		target = "allow_always"
	case "decline", "deny", "reject", "rejected", "no":
		target = "deny"
	default:
		if preferAlways {
			target = "allow_always"
		}
	}
	for _, option := range options {
		candidates := []string{
			strings.ToLower(strings.TrimSpace(option.OptionID)),
			strings.ToLower(strings.TrimSpace(option.Kind)),
			strings.ToLower(strings.TrimSpace(option.Name)),
		}
		for _, candidate := range candidates {
			if candidate == "" {
				continue
			}
			switch target {
			case "allow_always":
				if strings.Contains(candidate, "always") {
					return acp.Selected(option.OptionID)
				}
			case "deny":
				if strings.Contains(candidate, "deny") || strings.Contains(candidate, "decline") || strings.Contains(candidate, "reject") {
					return acp.Selected(option.OptionID)
				}
			default:
				if strings.Contains(candidate, "once") || strings.Contains(candidate, "allow") || strings.Contains(candidate, "accept") || strings.Contains(candidate, "approve") {
					return acp.Selected(option.OptionID)
				}
			}
		}
	}
	if len(options) > 0 {
		switch target {
		case "allow_always":
			if len(options) > 1 {
				return acp.Selected(options[1].OptionID)
			}
		case "deny":
			return acp.Selected(options[len(options)-1].OptionID)
		}
		return acp.Selected(options[0].OptionID)
	}
	return acp.CancelledOutcome()
}

func truthyApprovalFlag(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "always":
			return true
		}
	}
	return false
}

func mustMarshalJSON(value any) json.RawMessage {
	data, _ := json.Marshal(value)
	return data
}
