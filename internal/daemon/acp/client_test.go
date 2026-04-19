package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// --- test harness ---

type fakeAgent struct {
	t       *testing.T
	r       *frameReader
	w       *frameWriter
	inbound chan inFrame
	closed  chan struct{}

	stdinR  *io.PipeReader
	stdoutW *io.PipeWriter
}

func newFakeAgent(t *testing.T, stdinR *io.PipeReader, stdoutW *io.PipeWriter) *fakeAgent {
	t.Helper()
	a := &fakeAgent{
		t:       t,
		r:       newFrameReader(stdinR),
		w:       newFrameWriter(stdoutW),
		inbound: make(chan inFrame, 64),
		closed:  make(chan struct{}),
		stdinR:  stdinR,
		stdoutW: stdoutW,
	}
	go a.readLoop()
	return a
}

func (a *fakeAgent) readLoop() {
	defer close(a.closed)
	defer close(a.inbound)
	for {
		line, err := a.r.readFrame()
		if err != nil {
			return
		}
		var f inFrame
		if err := json.Unmarshal(line, &f); err != nil {
			a.t.Logf("fake agent: malformed frame: %v", err)
			continue
		}
		a.inbound <- f
	}
}

func (a *fakeAgent) expect(timeout time.Duration) (inFrame, bool) {
	select {
	case f, ok := <-a.inbound:
		return f, ok
	case <-time.After(timeout):
		return inFrame{}, false
	}
}

func (a *fakeAgent) respond(id json.RawMessage, result any) {
	a.t.Helper()
	if err := a.w.writeFrame(outResponse{
		JSONRPC: jsonRPCVersion,
		ID:      id,
		Result:  result,
	}); err != nil {
		a.t.Errorf("fake agent respond: %v", err)
	}
}

func (a *fakeAgent) respondError(id json.RawMessage, code int, msg string) {
	a.t.Helper()
	if err := a.w.writeFrame(outResponse{
		JSONRPC: jsonRPCVersion,
		ID:      id,
		Error:   &RPCError{Code: code, Message: msg},
	}); err != nil {
		a.t.Errorf("fake agent respondError: %v", err)
	}
}

func (a *fakeAgent) request(id int64, method string, params any) {
	a.t.Helper()
	if err := a.w.writeFrame(outRequest{
		JSONRPC: jsonRPCVersion,
		ID:      id,
		Method:  method,
		Params:  params,
	}); err != nil {
		a.t.Errorf("fake agent request: %v", err)
	}
}

func (a *fakeAgent) notify(method string, params any) {
	a.t.Helper()
	if err := a.w.writeFrame(outNotification{
		JSONRPC: jsonRPCVersion,
		Method:  method,
		Params:  params,
	}); err != nil {
		a.t.Errorf("fake agent notify: %v", err)
	}
}

func (a *fakeAgent) sendRaw(b []byte) {
	a.t.Helper()
	full := append(append([]byte(nil), b...), '\n')
	if _, err := a.stdoutW.Write(full); err != nil {
		a.t.Errorf("fake agent sendRaw: %v", err)
	}
}

func (a *fakeAgent) close() {
	_ = a.stdoutW.Close()
	_ = a.stdinR.Close()
}

// newTestClient wires a Client to an in-memory pipe pair and returns the
// client and a fake agent that drives the other side. The returned close
// function tears both sides down.
func newTestClient(t *testing.T) (*Client, *fakeAgent, func()) {
	t.Helper()
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	agent := newFakeAgent(t, stdinR, stdoutW)
	c := newClient(stdoutR, stdinW, time.Second, testLogger(t))
	cleanup := func() {
		_ = c.Close(context.Background())
		agent.close()
	}
	return c, agent, cleanup
}

func testLogger(t *testing.T) Logger {
	t.Helper()
	return func(format string, args ...any) {
		t.Logf("client: "+format, args...)
	}
}

// --- tests ---

// 3.2: framing happy path — requests terminate with \n, no embedded newlines,
// long lines (>64 KiB) round-trip.
func TestFramingLargePayloadRoundTrip(t *testing.T) {
	c, agent, cleanup := newTestClient(t)
	defer cleanup()

	big := strings.Repeat("x", 200*1024) // >64 KiB
	done := make(chan error, 1)
	var result struct {
		Echo string `json:"echo"`
	}
	go func() {
		done <- c.Call(context.Background(), "test/echo", map[string]string{"payload": big}, &result)
	}()

	f, ok := agent.expect(2 * time.Second)
	if !ok {
		t.Fatal("agent did not receive request")
	}
	if f.Method != "test/echo" {
		t.Fatalf("unexpected method %q", f.Method)
	}
	var params struct {
		Payload string `json:"payload"`
	}
	if err := json.Unmarshal(f.Params, &params); err != nil {
		t.Fatalf("decode params: %v", err)
	}
	if params.Payload != big {
		t.Fatalf("payload mangled: got len=%d want len=%d", len(params.Payload), len(big))
	}
	agent.respond(f.ID, map[string]string{"echo": big})

	if err := <-done; err != nil {
		t.Fatalf("call error: %v", err)
	}
	if result.Echo != big {
		t.Fatalf("response mangled: got len=%d want len=%d", len(result.Echo), len(big))
	}
}

// String payloads containing newlines must not desync the peer: JSON encoding
// escapes them so the framed output has exactly one terminator.
func TestFrameWriterEncodesStringNewlines(t *testing.T) {
	var buf strings.Builder
	w := newFrameWriter(&buf)
	if err := w.writeFrame(map[string]string{"s": "line1\nline2"}); err != nil {
		t.Fatalf("writeFrame: %v", err)
	}
	got := buf.String()
	if strings.Count(got, "\n") != 1 {
		t.Fatalf("expected exactly one newline, got %d in %q", strings.Count(got, "\n"), got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Fatalf("frame not terminated with \\n: %q", got)
	}
}

// 3.3: concurrent request/response correlation.
func TestConcurrentCallsCorrelate(t *testing.T) {
	c, agent, cleanup := newTestClient(t)
	defer cleanup()

	// Agent echoes each request's params as its result.
	go func() {
		for f := range agent.inbound {
			agent.respond(f.ID, json.RawMessage(f.Params))
		}
	}()

	const N = 32
	var wg sync.WaitGroup
	errs := make([]error, N)
	results := make([]int, N)
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var out struct {
				N int `json:"n"`
			}
			err := c.Call(context.Background(), "test/concurrent",
				map[string]int{"n": i}, &out)
			errs[i] = err
			results[i] = out.N
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if results[i] != i {
			t.Fatalf("call %d got result %d", i, results[i])
		}
	}
}

// 3.4a: malformed JSON on the wire is logged but does not crash the client;
// subsequent messages continue to be processed.
func TestMalformedFrameDoesNotTearDown(t *testing.T) {
	c, agent, cleanup := newTestClient(t)
	defer cleanup()

	agent.sendRaw([]byte("this is not json at all"))

	// Now send a legitimate notification and verify the subscriber still
	// receives it — i.e. the read loop is alive.
	sub := c.Subscribe()
	agent.notify("session/update", map[string]any{
		"sessionId": "abc",
		"update":    map[string]string{"sessionUpdate": "plan"},
	})
	select {
	case n := <-sub:
		if n.Method != "session/update" {
			t.Fatalf("unexpected notification %q", n.Method)
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber did not receive notification after malformed frame")
	}
}

// 3.4b: JSON-RPC error responses are surfaced to the originating caller.
func TestJSONRPCErrorReturnedToCaller(t *testing.T) {
	c, agent, cleanup := newTestClient(t)
	defer cleanup()

	done := make(chan error, 1)
	go func() {
		done <- c.Call(context.Background(), "test/fails", nil, nil)
	}()

	f, ok := agent.expect(time.Second)
	if !ok {
		t.Fatal("no request")
	}
	agent.respondError(f.ID, ErrorCodeInvalidParams, "bad params")

	err := <-done
	var rpcErr *RPCError
	if !errors.As(err, &rpcErr) {
		t.Fatalf("expected *RPCError, got %T: %v", err, err)
	}
	if rpcErr.Code != ErrorCodeInvalidParams || rpcErr.Message != "bad params" {
		t.Fatalf("wrong rpc error: %+v", rpcErr)
	}
}

// Ctx cancellation returns ctx.Err and subsequent late responses are
// discarded without panicking.
func TestCallContextCancellation(t *testing.T) {
	c, agent, cleanup := newTestClient(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() { done <- c.Call(ctx, "test/slow", nil, nil) }()

	f, ok := agent.expect(time.Second)
	if !ok {
		t.Fatal("no request")
	}
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("call did not return after cancel")
	}

	// Late response should be harmlessly discarded.
	agent.respond(f.ID, nil)
	time.Sleep(50 * time.Millisecond)
}

// 3.5: two subscribers both observe a notification; a stalled subscriber does
// not block the read loop.
func TestNotificationFanOut(t *testing.T) {
	c, agent, cleanup := newTestClient(t)
	defer cleanup()

	a := c.Subscribe()
	b := c.Subscribe()

	agent.notify("session/update", map[string]any{
		"sessionId": "s1",
		"update":    map[string]string{"sessionUpdate": "plan"},
	})

	for _, ch := range []<-chan Notification{a, b} {
		select {
		case n := <-ch:
			if n.Method != "session/update" {
				t.Fatalf("unexpected method %q", n.Method)
			}
		case <-time.After(time.Second):
			t.Fatal("subscriber did not receive notification")
		}
	}
}

func TestStalledSubscriberDoesNotBlockReadLoop(t *testing.T) {
	c, agent, cleanup := newTestClient(t)
	defer cleanup()

	stalled := c.Subscribe()
	_ = stalled // never read
	live := c.Subscribe()

	// Blast more notifications than the subscriber buffer can hold.
	for i := 0; i < subscriberBufferSize*3; i++ {
		agent.notify("session/update", map[string]any{
			"sessionId": fmt.Sprintf("s%d", i),
			"update":    map[string]any{"sessionUpdate": "plan"},
		})
	}

	// Then send one more — the live subscriber must still be receiving.
	agent.notify("session/update", map[string]any{
		"sessionId": "final",
		"update":    map[string]any{"sessionUpdate": "plan"},
	})

	deadline := time.After(2 * time.Second)
	seen := 0
	for seen < subscriberBufferSize {
		select {
		case <-live:
			seen++
		case <-deadline:
			t.Fatalf("live subscriber got only %d notifications; read loop likely stalled", seen)
		}
	}
}

// 3.6: unknown sessionUpdate discriminator is preserved as raw JSON.
func TestUnknownSessionUpdatePreservesRaw(t *testing.T) {
	c, agent, cleanup := newTestClient(t)
	defer cleanup()

	sub := c.Subscribe()
	raw := map[string]any{
		"sessionId": "s1",
		"update": map[string]any{
			"sessionUpdate": "future_variant",
			"customField":   "hello",
		},
	}
	agent.notify("session/update", raw)

	var n Notification
	select {
	case n = <-sub:
	case <-time.After(time.Second):
		t.Fatal("no notification")
	}
	decoded, err := DecodeSessionUpdate(n.Params)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.SessionUpdate != "future_variant" {
		t.Fatalf("wrong discriminator: %q", decoded.SessionUpdate)
	}
	if len(decoded.Raw) == 0 {
		t.Fatal("raw payload lost")
	}
	if !strings.Contains(string(decoded.Raw), "customField") {
		t.Fatalf("raw payload missing customField: %s", decoded.Raw)
	}
	if decoded.AgentMessageChunk != nil || decoded.Plan != nil || decoded.ToolCall != nil {
		t.Fatal("unknown variant populated a typed field")
	}
}

// 3.7a: registered handler receives request params and its return value is
// sent back under the original id.
func TestIncomingRequestDispatch(t *testing.T) {
	c, agent, cleanup := newTestClient(t)
	defer cleanup()

	c.RegisterHandler(MethodRequestPermission, HandlePermission(
		func(ctx context.Context, params RequestPermissionParams) (RequestPermissionOutcome, error) {
			if params.SessionID != "s1" {
				return RequestPermissionOutcome{}, fmt.Errorf("unexpected session id %q", params.SessionID)
			}
			if len(params.Options) == 0 {
				return RequestPermissionOutcome{}, errors.New("no options")
			}
			return Selected(params.Options[0].OptionID), nil
		}))

	agent.request(42, MethodRequestPermission, RequestPermissionParams{
		SessionID: "s1",
		ToolCall:  ToolCall{ToolCallID: "tc1"},
		Options:   []PermissionOption{{OptionID: "allow_once", Name: "Allow once"}},
	})

	f, ok := agent.expect(2 * time.Second)
	if !ok {
		t.Fatal("no response from client")
	}
	if string(f.ID) != "42" {
		t.Fatalf("wrong id %s", f.ID)
	}
	if f.Error != nil {
		t.Fatalf("handler returned error: %+v", f.Error)
	}
	var res RequestPermissionResult
	if err := json.Unmarshal(f.Result, &res); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if res.Outcome.Outcome != PermissionOutcomeSelected || res.Outcome.OptionID != "allow_once" {
		t.Fatalf("wrong outcome: %+v", res.Outcome)
	}
}

// Close cancels the context handed to an in-flight handler so blocked
// handlers (e.g. approval prompts waiting for user input) return promptly.
func TestHandlerContextCancelledOnClose(t *testing.T) {
	c, agent, _ := newTestClient(t)

	released := make(chan struct{})
	c.RegisterHandler("test/slow", func(ctx context.Context, _ json.RawMessage) (any, error) {
		<-ctx.Done()
		close(released)
		return nil, ctx.Err()
	})

	agent.request(1, "test/slow", nil)
	// Give the dispatch goroutine a moment to enter the handler.
	time.Sleep(50 * time.Millisecond)

	if err := c.Close(context.Background()); err != nil {
		t.Fatalf("close err: %v", err)
	}

	select {
	case <-released:
	case <-time.After(time.Second):
		t.Fatal("handler did not observe Close via context")
	}

	agent.close()
}

// A panicking handler responds to the agent with an internal error instead of
// hanging the request id forever.
func TestHandlerPanicRecovered(t *testing.T) {
	c, agent, cleanup := newTestClient(t)
	defer cleanup()

	c.RegisterHandler("test/boom", func(context.Context, json.RawMessage) (any, error) {
		panic("boom")
	})

	agent.request(99, "test/boom", nil)

	f, ok := agent.expect(2 * time.Second)
	if !ok {
		t.Fatal("no response from panicking handler")
	}
	if f.Error == nil || f.Error.Code != ErrorCodeInternalError {
		t.Fatalf("expected internal error, got %+v", f.Error)
	}
	if !strings.Contains(f.Error.Message, "panic") {
		t.Fatalf("error message lacks panic context: %q", f.Error.Message)
	}
}

// 3.7b: unregistered method returns method_not_found.
func TestUnregisteredMethodReturnsMethodNotFound(t *testing.T) {
	_, agent, cleanup := newTestClient(t)
	defer cleanup()

	agent.request(7, "fs/read_text_file", map[string]string{"path": "/etc/hosts"})

	f, ok := agent.expect(2 * time.Second)
	if !ok {
		t.Fatal("no response")
	}
	if string(f.ID) != "7" {
		t.Fatalf("wrong id %s", f.ID)
	}
	if f.Error == nil {
		t.Fatal("expected error response")
	}
	if f.Error.Code != ErrorCodeMethodNotFound {
		t.Fatalf("wrong error code %d", f.Error.Code)
	}
}

// 3.8: initialize fails loudly on protocol-version mismatch.
func TestInitializeVersionMismatch(t *testing.T) {
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	agent := newFakeAgent(t, stdinR, stdoutW)
	c := newClient(stdoutR, stdinW, time.Second, testLogger(t))
	t.Cleanup(func() {
		_ = c.Close(context.Background())
		agent.close()
	})

	go func() {
		f, ok := agent.expect(2 * time.Second)
		if !ok {
			return
		}
		agent.respond(f.ID, InitializeResult{
			ProtocolVersion: 99,
			AgentInfo:       ImplementationInfo{Name: "fake"},
		})
	}()

	err := c.initialize(context.Background(), StartOptions{
		ClientInfo:      ImplementationInfo{Name: "archon-test"},
		ProtocolVersion: ProtocolVersion1,
	})
	if err == nil {
		t.Fatal("expected version-mismatch error")
	}
	if !strings.Contains(err.Error(), "protocol version") {
		t.Fatalf("error lacks descriptive text: %v", err)
	}
}

// 3.9: session/cancel notification resolves an outstanding session/prompt
// call with stopReason: "cancelled".
func TestSessionCancelResolvesPromptAsCancelled(t *testing.T) {
	c, agent, cleanup := newTestClient(t)
	defer cleanup()

	done := make(chan error, 1)
	var result PromptResult
	go func() {
		done <- c.Call(context.Background(), MethodSessionPrompt,
			PromptParams{SessionID: "s1", Prompt: []ContentBlock{{Type: "text", Text: "hi"}}},
			&result)
	}()

	prompt, ok := agent.expect(2 * time.Second)
	if !ok {
		t.Fatal("prompt never arrived")
	}

	if err := c.Notify(MethodSessionCancel, CancelParams{SessionID: "s1"}); err != nil {
		t.Fatalf("notify cancel: %v", err)
	}

	cancel, ok := agent.expect(2 * time.Second)
	if !ok || cancel.Method != MethodSessionCancel {
		t.Fatalf("expected session/cancel, got %+v", cancel)
	}

	// Agent responds to the prompt with cancelled stop reason.
	agent.respond(prompt.ID, PromptResult{StopReason: StopReasonCancelled})

	if err := <-done; err != nil {
		t.Fatalf("prompt call err: %v", err)
	}
	if result.StopReason != StopReasonCancelled {
		t.Fatalf("wrong stop reason %q", result.StopReason)
	}
}

// 3.10a: Close without a cmd closes stdin and returns promptly.
func TestCloseWithoutCmd(t *testing.T) {
	c, agent, _ := newTestClient(t)
	defer agent.close()

	done := make(chan error, 1)
	go func() { done <- c.Close(context.Background()) }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("close err: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not return")
	}

	// Subsequent Close is idempotent.
	if err := c.Close(context.Background()); err != nil {
		t.Fatalf("second close err: %v", err)
	}

	// Calls after close fail with ErrClientClosed.
	if err := c.Call(context.Background(), "test/anything", nil, nil); !errors.Is(err, ErrClientClosed) {
		t.Fatalf("expected ErrClientClosed, got %v", err)
	}
}

// 3.10b: Close kills an unresponsive subprocess once the timeout elapses.
func TestCloseKillsUnresponsiveSubprocess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-only subprocess test")
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	// `exec sleep 30` replaces the shell with sleep so stdin closure does not
	// naturally terminate the process.
	cmd := exec.Command("sh", "-c", "exec sleep 30")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	c := newClient(stdout, stdin, 200*time.Millisecond, testLogger(t))
	c.cmd = cmd

	start := time.Now()
	err = c.Close(context.Background())
	elapsed := time.Since(start)

	if elapsed > 5*time.Second {
		t.Fatalf("Close too slow: %v", elapsed)
	}
	if err == nil || !strings.Contains(err.Error(), "killed") {
		t.Fatalf("expected kill error, got %v", err)
	}

	if state := cmd.ProcessState; state == nil || !state.Exited() {
		// If Wait has not reported, it should by now — fetch it.
		_ = cmd.Wait()
	}
}
