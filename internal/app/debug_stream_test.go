package app

import (
	"testing"
	"time"

	"control/internal/types"
)

type testDebugFormatter struct{}

func (testDebugFormatter) Format(line string) (string, bool) {
	return "fmt:" + line, true
}

type testDebugFormatWorker struct {
	closed   bool
	requests []debugFormatRequest
}

func (w *testDebugFormatWorker) Enqueue(req debugFormatRequest) {
	w.requests = append(w.requests, req)
}

func (w *testDebugFormatWorker) Drain(apply func(debugFormatResult)) {
	for _, req := range w.requests {
		apply(debugFormatResult{
			generation: req.generation,
			lineID:     req.lineID,
			formatted:  "wrk:" + req.line,
			changed:    true,
		})
	}
	w.requests = nil
}

func (w *testDebugFormatWorker) Close() {
	w.closed = true
}

func TestDebugStreamControllerConsumeTickAccumulatesAndCloses(t *testing.T) {
	controller := NewDebugStreamController(DebugStreamRetentionPolicy{MaxLines: 2, MaxBytes: 1024}, 10)
	ch := make(chan types.DebugEvent, 4)
	controller.SetStream(ch, nil)

	ch <- types.DebugEvent{Chunk: "first\nsecond\n"}
	lines, changed, closed := controller.ConsumeTick()
	if closed {
		t.Fatalf("expected open stream")
	}
	if !changed {
		t.Fatalf("expected changed=true on chunk consumption")
	}
	if len(lines) != 2 || lines[0] != "first" || lines[1] != "second" {
		t.Fatalf("unexpected lines after first tick: %#v", lines)
	}

	ch <- types.DebugEvent{Chunk: "third\n"}
	lines, changed, closed = controller.ConsumeTick()
	if closed {
		t.Fatalf("expected stream to still be open")
	}
	if !changed {
		t.Fatalf("expected changed=true after second chunk")
	}
	if len(lines) != 2 || lines[0] != "second" || lines[1] != "third" {
		t.Fatalf("expected max-lines trimming, got %#v", lines)
	}

	close(ch)
	_, changed, closed = controller.ConsumeTick()
	if !closed {
		t.Fatalf("expected closed=true when channel is closed")
	}
	if changed {
		t.Fatalf("expected changed=false on close without new chunks")
	}
	if controller.HasStream() {
		t.Fatalf("expected stream to be detached when closed")
	}
}

func TestDebugStreamControllerResetCancelsAndClearsState(t *testing.T) {
	controller := NewDebugStreamController(DebugStreamRetentionPolicy{MaxLines: 10, MaxBytes: 1024}, 10)
	called := 0
	controller.lines = []string{"line"}
	controller.pending = "partial"
	controller.SetStream(make(chan types.DebugEvent), func() {
		called++
	})

	controller.Reset()

	if called != 1 {
		t.Fatalf("expected cancel function to be called once, got %d", called)
	}
	if controller.HasStream() {
		t.Fatalf("expected stream to be cleared")
	}
	if len(controller.Lines()) != 0 {
		t.Fatalf("expected lines to be cleared, got %#v", controller.Lines())
	}
	if controller.pending != "" {
		t.Fatalf("expected pending buffer to be cleared, got %q", controller.pending)
	}
}

func TestDebugStreamControllerTrimsByBytes(t *testing.T) {
	controller := NewDebugStreamController(DebugStreamRetentionPolicy{MaxLines: 10, MaxBytes: 7}, 10)
	ch := make(chan types.DebugEvent, 1)
	controller.SetStream(ch, nil)

	ch <- types.DebugEvent{Chunk: "abcd\nefgh\n"}
	lines, changed, _ := controller.ConsumeTick()
	if !changed {
		t.Fatalf("expected changed=true on chunk consumption")
	}
	if len(lines) != 1 || lines[0] != "efgh" {
		t.Fatalf("expected byte-bound trimming to retain latest line, got %#v", lines)
	}
	if got := controller.Content(); got != "efgh" {
		t.Fatalf("expected cached content to match trimmed lines, got %q", got)
	}
}

func TestDebugStreamControllerPrettyFormatsJSONAsynchronously(t *testing.T) {
	controller := NewDebugStreamController(DebugStreamRetentionPolicy{MaxLines: 10, MaxBytes: 4096}, 10)
	ch := make(chan types.DebugEvent, 1)
	controller.SetStream(ch, nil)

	ch <- types.DebugEvent{Chunk: "{\"a\":1,\"b\":{\"c\":2}}\n"}
	_, changed, _ := controller.ConsumeTick()
	if !changed {
		t.Fatalf("expected first tick to consume line")
	}

	deadline := time.Now().Add(200 * time.Millisecond)
	for {
		content := controller.Content()
		if content != "" && content != "{\"a\":1,\"b\":{\"c\":2}}" {
			if content == "{\n  \"a\": 1,\n  \"b\": {\n    \"c\": 2\n  }\n}" {
				return
			}
			t.Fatalf("unexpected formatted content %q", content)
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected pretty formatted JSON content, got %q", content)
		}
		controller.ConsumeTick()
		time.Sleep(5 * time.Millisecond)
	}
}

func TestDebugStreamControllerFallsBackForNonJSONLines(t *testing.T) {
	controller := NewDebugStreamController(DebugStreamRetentionPolicy{MaxLines: 10, MaxBytes: 4096}, 10)
	ch := make(chan types.DebugEvent, 1)
	controller.SetStream(ch, nil)

	ch <- types.DebugEvent{Chunk: "plain text line\n"}
	_, changed, _ := controller.ConsumeTick()
	if !changed {
		t.Fatalf("expected first tick to consume non-json line")
	}
	if got := controller.Content(); got != "plain text line" {
		t.Fatalf("expected non-json line to remain unchanged, got %q", got)
	}
}

func TestDebugStreamControllerOptionsUseInjectedFormatter(t *testing.T) {
	controller := NewDebugStreamController(
		DebugStreamRetentionPolicy{MaxLines: 10, MaxBytes: 4096},
		10,
		WithDebugLineFormatter(testDebugFormatter{}),
	)
	ch := make(chan types.DebugEvent, 1)
	controller.SetStream(ch, nil)

	ch <- types.DebugEvent{Chunk: "hello\n"}
	_, _, _ = controller.ConsumeTick()

	deadline := time.Now().Add(200 * time.Millisecond)
	for {
		if got := controller.Content(); got == "fmt:hello" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected injected formatter output, got %q", controller.Content())
		}
		controller.ConsumeTick()
		time.Sleep(5 * time.Millisecond)
	}
}

func TestDebugStreamControllerCloseClosesWorker(t *testing.T) {
	worker := &testDebugFormatWorker{}
	controller := NewDebugStreamController(
		DebugStreamRetentionPolicy{MaxLines: 10, MaxBytes: 4096},
		10,
		WithDebugFormatWorker(worker),
	)
	ch := make(chan types.DebugEvent, 1)
	controller.SetStream(ch, nil)
	ch <- types.DebugEvent{Chunk: "hello\n"}
	_, _, _ = controller.ConsumeTick()

	controller.Close()
	if !worker.closed {
		t.Fatalf("expected custom worker to be closed")
	}
}

func TestAsyncDebugFormatWorkerCloseIdempotentAndStopsEnqueue(t *testing.T) {
	worker := newAsyncDebugFormatWorker(testDebugFormatter{}, 2)
	worker.Close()
	worker.Close()

	worker.Enqueue(debugFormatRequest{generation: 1, lineID: 1, line: "hello"})
	called := false
	worker.Drain(func(debugFormatResult) {
		called = true
	})
	if called {
		t.Fatalf("expected closed worker to produce no drained results")
	}
}

func TestDebugStreamControllerOptionsCoverQueueAndMaxBytes(t *testing.T) {
	controller := NewDebugStreamController(
		DebugStreamRetentionPolicy{MaxLines: 10, MaxBytes: 4096},
		10,
		WithDebugFormatQueueSize(1),
		WithDebugFormatMaxBytes(5),
	)
	ch := make(chan types.DebugEvent, 1)
	controller.SetStream(ch, nil)

	// Valid JSON but above max-bytes threshold: should remain raw.
	ch <- types.DebugEvent{Chunk: "{\"a\":1234}\n"}
	_, changed, _ := controller.ConsumeTick()
	if !changed {
		t.Fatalf("expected stream update")
	}
	if got := controller.Content(); got != "{\"a\":1234}" {
		t.Fatalf("expected JSON to remain raw due to max-bytes cap, got %q", got)
	}
}
