package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

const (
	jsonRPCVersion       = "2.0"
	defaultCloseTimeout  = 5 * time.Second
	subscriberBufferSize = 64
)

// ErrClientClosed is returned from Call/Notify after Close has completed or
// after the read loop has terminated.
var ErrClientClosed = errors.New("acp: client closed")

// Logger is a minimal structured logger. It receives free-form debug messages
// about dropped notifications, parse failures, and unexpected responses. It
// MUST be non-blocking; the client calls it from the read loop.
type Logger func(format string, args ...any)

// StartOptions configures the subprocess Start will spawn and the initialize
// handshake it performs.
type StartOptions struct {
	// Command is the path (or PATH-resolvable name) of the agent binary.
	Command string
	// Args are passed to the agent in argv (exec.Command semantics).
	Args []string
	// Env overrides the subprocess environment. Nil means inherit os.Environ.
	Env []string
	// Cwd sets the subprocess working directory. Empty means inherit.
	Cwd string

	// Stderr receives the agent's stderr stream. Nil discards it. The client
	// never interprets stderr as protocol data.
	Stderr io.Writer

	// ClientInfo is advertised to the agent during initialize.
	ClientInfo ImplementationInfo
	// ClientCapabilities is advertised to the agent during initialize.
	ClientCapabilities ClientCapabilities
	// ProtocolVersion the client advertises. Defaults to ProtocolVersion1.
	ProtocolVersion int

	// InitializeTimeout bounds how long Start waits for the initialize
	// response before aborting. Zero defers to ctx only.
	InitializeTimeout time.Duration
	// CloseTimeout bounds how long Close waits for the subprocess to exit
	// after stdin is closed before sending a kill signal. Zero uses 5s.
	CloseTimeout time.Duration

	// Logger receives debug messages (dropped notifications, parse failures).
	// Nil selects a no-op logger.
	Logger Logger
}

// Client is a single-connection ACP JSON-RPC peer. A Client owns one agent
// subprocess (when created via Start) or a pair of attached streams (via the
// internal newClient constructor used in tests). It is safe for concurrent
// use.
type Client struct {
	writer *frameWriter
	reader *frameReader

	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser // may be nil in test mode

	closeTimeout time.Duration
	logger       Logger

	nextID atomic.Int64

	mu       sync.Mutex
	pending  map[int64]chan rpcResponse
	handlers map[string]RequestHandler
	stopped  bool

	subsMu sync.Mutex
	subs   map[*subscription]struct{}

	capsMu    sync.RWMutex
	agentCaps AgentCapabilities
	agentInfo ImplementationInfo

	closeOnce sync.Once
	readDone  chan struct{}
	closed    chan struct{}
	closeErr  error

	// shutdownCtx is cancelled by Close so in-flight handler goroutines can
	// observe shutdown and return.
	shutdownCtx    context.Context
	cancelShutdown context.CancelFunc
}

type rpcResponse struct {
	result json.RawMessage
	rpcErr *RPCError
	err    error
}

type subscription struct {
	ch      chan Notification
	dropped atomic.Int64
}

type inFrame struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

type outRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type outNotification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type outResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// Start launches the configured agent subprocess, wires up framing, runs the
// initialize handshake, and returns a ready-to-use Client. On failure the
// subprocess is torn down before returning.
func Start(ctx context.Context, opts StartOptions) (*Client, error) {
	if opts.Command == "" {
		return nil, errors.New("acp: StartOptions.Command is required")
	}

	cmd := exec.CommandContext(ctx, opts.Command, opts.Args...)
	if len(opts.Env) > 0 {
		cmd.Env = opts.Env
	}
	if opts.Cwd != "" {
		cmd.Dir = opts.Cwd
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("acp: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("acp: stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, fmt.Errorf("acp: stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
		return nil, fmt.Errorf("acp: start %s: %w", opts.Command, err)
	}

	if opts.Stderr != nil {
		go func() { _, _ = io.Copy(opts.Stderr, stderr) }()
	} else {
		go func() { _, _ = io.Copy(io.Discard, stderr) }()
	}

	c := newClient(stdout, stdin, opts.CloseTimeout, opts.Logger)
	c.cmd = cmd
	c.stdout = stdout

	if err := c.initialize(ctx, opts); err != nil {
		_ = c.Close(context.Background())
		return nil, err
	}
	return c, nil
}

func newClient(stdout io.Reader, stdin io.WriteCloser, closeTimeout time.Duration, logger Logger) *Client {
	if closeTimeout <= 0 {
		closeTimeout = defaultCloseTimeout
	}
	if logger == nil {
		logger = func(string, ...any) {}
	}
	shutdownCtx, cancel := context.WithCancel(context.Background())
	c := &Client{
		writer:         newFrameWriter(stdin),
		reader:         newFrameReader(stdout),
		stdin:          stdin,
		closeTimeout:   closeTimeout,
		logger:         logger,
		pending:        make(map[int64]chan rpcResponse),
		handlers:       make(map[string]RequestHandler),
		subs:           make(map[*subscription]struct{}),
		readDone:       make(chan struct{}),
		closed:         make(chan struct{}),
		shutdownCtx:    shutdownCtx,
		cancelShutdown: cancel,
	}
	go c.readLoop()
	return c
}

func (c *Client) initialize(ctx context.Context, opts StartOptions) error {
	pv := opts.ProtocolVersion
	if pv == 0 {
		pv = ProtocolVersion1
	}
	params := InitializeParams{
		ProtocolVersion:    pv,
		ClientCapabilities: opts.ClientCapabilities,
		ClientInfo:         opts.ClientInfo,
	}

	initCtx := ctx
	if opts.InitializeTimeout > 0 {
		var cancel context.CancelFunc
		initCtx, cancel = context.WithTimeout(ctx, opts.InitializeTimeout)
		defer cancel()
	}

	var result InitializeResult
	if err := c.Call(initCtx, MethodInitialize, params, &result); err != nil {
		return fmt.Errorf("acp: initialize: %w", err)
	}
	if result.ProtocolVersion != pv {
		return fmt.Errorf("acp: protocol version mismatch: client=%d agent=%d", pv, result.ProtocolVersion)
	}
	c.capsMu.Lock()
	c.agentCaps = result.AgentCapabilities
	c.agentInfo = result.AgentInfo
	c.capsMu.Unlock()
	return nil
}

// AgentCapabilities returns the capabilities the agent reported during
// initialize. The zero value is returned before initialize completes.
func (c *Client) AgentCapabilities() AgentCapabilities {
	c.capsMu.RLock()
	defer c.capsMu.RUnlock()
	return c.agentCaps
}

// AgentInfo returns the agent's implementation info from initialize.
func (c *Client) AgentInfo() ImplementationInfo {
	c.capsMu.RLock()
	defer c.capsMu.RUnlock()
	return c.agentInfo
}

// Done is closed when the read loop has exited. Callers may select on it to
// detect a disconnected or crashed agent.
func (c *Client) Done() <-chan struct{} { return c.readDone }

// Call issues a JSON-RPC request, blocks until the agent responds or ctx is
// cancelled, and decodes the result into out (if non-nil). An RPC error from
// the agent is returned as *RPCError.
func (c *Client) Call(ctx context.Context, method string, params any, out any) error {
	id := c.nextID.Add(1)
	ch := make(chan rpcResponse, 1)

	c.mu.Lock()
	if c.stopped {
		c.mu.Unlock()
		return ErrClientClosed
	}
	c.pending[id] = ch
	c.mu.Unlock()

	req := outRequest{JSONRPC: jsonRPCVersion, ID: id, Method: method, Params: params}
	if err := c.writer.writeFrame(req); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return fmt.Errorf("acp: send %s: %w", method, err)
	}

	select {
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return ctx.Err()
	case resp := <-ch:
		if resp.err != nil {
			return resp.err
		}
		if resp.rpcErr != nil {
			return resp.rpcErr
		}
		if out == nil || len(resp.result) == 0 {
			return nil
		}
		if err := json.Unmarshal(resp.result, out); err != nil {
			return fmt.Errorf("acp: decode %s result: %w", method, err)
		}
		return nil
	}
}

// Notify sends a JSON-RPC notification (no id, no response expected). Returns
// once the frame has been written.
func (c *Client) Notify(method string, params any) error {
	c.mu.Lock()
	if c.stopped {
		c.mu.Unlock()
		return ErrClientClosed
	}
	c.mu.Unlock()

	return c.writer.writeFrame(outNotification{
		JSONRPC: jsonRPCVersion,
		Method:  method,
		Params:  params,
	})
}

// Subscribe returns a channel that receives every inbound notification
// delivered to the client. The returned channel is buffered; when full, the
// client drops the OLDEST buffered notification to make room for the newest
// (and logs a debug message). Call Unsubscribe to stop delivery.
func (c *Client) Subscribe() <-chan Notification {
	sub := &subscription{ch: make(chan Notification, subscriberBufferSize)}
	c.subsMu.Lock()
	c.subs[sub] = struct{}{}
	c.subsMu.Unlock()
	return sub.ch
}

// Unsubscribe removes a previously subscribed channel. The channel is closed
// after removal so ranged receivers terminate cleanly.
func (c *Client) Unsubscribe(ch <-chan Notification) {
	c.subsMu.Lock()
	defer c.subsMu.Unlock()
	for sub := range c.subs {
		if sub.ch == ch {
			delete(c.subs, sub)
			close(sub.ch)
			return
		}
	}
}

// RegisterHandler installs a handler for an agent→client request method.
// Registering a new handler for an existing method replaces the previous
// registration. Pass nil to unregister.
func (c *Client) RegisterHandler(method string, h RequestHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if h == nil {
		delete(c.handlers, method)
		return
	}
	c.handlers[method] = h
}

// Close closes the agent's stdin, waits for the read loop and subprocess to
// exit, and kills the subprocess if it does not exit within the configured
// CloseTimeout. Safe to call more than once; subsequent calls return the
// first Close's error.
func (c *Client) Close(ctx context.Context) error {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.stopped = true
		c.mu.Unlock()

		c.cancelShutdown()
		_ = c.stdin.Close()

		var procExited chan error
		if c.cmd != nil {
			procExited = make(chan error, 1)
			go func() { procExited <- c.cmd.Wait() }()
		}

		deadline := time.After(c.closeTimeout)

		// First, wait for the read loop to exit, the deadline to fire, or the
		// caller's context to cancel. After any of those, if we own a
		// subprocess we separately ensure it is gone.
		select {
		case <-c.readDone:
		case <-deadline:
		case <-ctx.Done():
		}

		if procExited != nil {
			select {
			case err := <-procExited:
				if err != nil && !isExpectedExit(err) {
					c.closeErr = err
				}
			default:
				_ = c.cmd.Process.Kill()
				<-procExited
				if ctx.Err() != nil {
					c.closeErr = ctx.Err()
				} else {
					c.closeErr = errors.New("acp: agent did not exit before close timeout; killed")
				}
			}
		}

		c.drainPending(ErrClientClosed)
		c.closeSubscribers()
		close(c.closed)
	})
	<-c.closed
	return c.closeErr
}

func isExpectedExit(err error) bool {
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr)
}

func (c *Client) readLoop() {
	defer close(c.readDone)
	for {
		line, err := c.reader.readFrame()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				c.logger("acp: read loop exiting: %v", err)
			}
			c.drainPending(fmt.Errorf("acp: connection closed: %w", err))
			return
		}
		var frame inFrame
		if err := json.Unmarshal(line, &frame); err != nil {
			c.logger("acp: malformed frame (%d bytes): %v", len(line), err)
			continue
		}
		c.dispatch(frame)
	}
}

func (c *Client) dispatch(frame inFrame) {
	switch {
	case frame.Method != "" && hasID(frame.ID):
		c.handleIncomingRequest(frame)
	case frame.Method != "":
		c.fanoutNotification(Notification{Method: frame.Method, Params: frame.Params})
	case hasID(frame.ID):
		c.deliverResponse(frame)
	default:
		c.logger("acp: discarding frame with no method and no id")
	}
}

func hasID(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	// JSON-RPC spec forbids null ids on responses, but be tolerant on input.
	return string(raw) != "null"
}

func (c *Client) deliverResponse(frame inFrame) {
	var id int64
	if err := json.Unmarshal(frame.ID, &id); err != nil {
		c.logger("acp: response with non-numeric id %s dropped", string(frame.ID))
		return
	}
	c.mu.Lock()
	ch, ok := c.pending[id]
	if ok {
		delete(c.pending, id)
	}
	c.mu.Unlock()
	if !ok {
		c.logger("acp: response for unknown id %d", id)
		return
	}
	ch <- rpcResponse{result: frame.Result, rpcErr: frame.Error}
}

func (c *Client) handleIncomingRequest(frame inFrame) {
	c.mu.Lock()
	handler, ok := c.handlers[frame.Method]
	c.mu.Unlock()

	id := append(json.RawMessage(nil), frame.ID...)

	if !ok {
		_ = c.writer.writeFrame(outResponse{
			JSONRPC: jsonRPCVersion,
			ID:      id,
			Error: &RPCError{
				Code:    ErrorCodeMethodNotFound,
				Message: "method not found: " + frame.Method,
			},
		})
		return
	}

	// Dispatch handlers on their own goroutine so slow handlers do not stall
	// the read loop. The shutdown context lets handlers observe Close and
	// return promptly instead of blocking on abandoned work.
	go func() {
		resp := outResponse{JSONRPC: jsonRPCVersion, ID: id}
		defer func() {
			if r := recover(); r != nil {
				c.logger("acp: handler panic for %s: %v", frame.Method, r)
				resp.Result = nil
				resp.Error = &RPCError{
					Code:    ErrorCodeInternalError,
					Message: fmt.Sprintf("handler panic: %v", r),
				}
			}
			if werr := c.writer.writeFrame(resp); werr != nil {
				c.logger("acp: failed to send response for %s: %v", frame.Method, werr)
			}
		}()

		result, err := handler(c.shutdownCtx, frame.Params)
		if err != nil {
			var rpcErr *RPCError
			if errors.As(err, &rpcErr) {
				resp.Error = rpcErr
			} else {
				resp.Error = &RPCError{Code: ErrorCodeInternalError, Message: err.Error()}
			}
			return
		}
		resp.Result = result
	}()
}

func (c *Client) fanoutNotification(n Notification) {
	c.subsMu.Lock()
	defer c.subsMu.Unlock()
	for sub := range c.subs {
		select {
		case sub.ch <- n:
			continue
		default:
		}
		// Buffer full — drop oldest, enqueue newest (best-effort).
		select {
		case <-sub.ch:
		default:
		}
		select {
		case sub.ch <- n:
		default:
		}
		count := sub.dropped.Add(1)
		c.logger("acp: slow subscriber dropped %d notifications (method=%s)", count, n.Method)
	}
}

func (c *Client) drainPending(err error) {
	c.mu.Lock()
	pending := c.pending
	c.pending = make(map[int64]chan rpcResponse)
	c.stopped = true
	c.mu.Unlock()
	for _, ch := range pending {
		ch <- rpcResponse{err: err}
	}
}

func (c *Client) closeSubscribers() {
	c.subsMu.Lock()
	defer c.subsMu.Unlock()
	for sub := range c.subs {
		close(sub.ch)
	}
	c.subs = make(map[*subscription]struct{})
}
