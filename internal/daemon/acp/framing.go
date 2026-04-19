package acp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
)

// maxLineBytes is the largest single JSON-RPC frame we accept on the wire. ACP
// permits arbitrarily large messages in practice (tool outputs, file contents),
// so we raise bufio's default 64 KiB scan buffer well beyond that.
const maxLineBytes = 8 * 1024 * 1024

// ErrEmbeddedNewline is returned when a marshaled frame contains a '\n'
// byte before the trailing terminator. The wire format reserves '\n' as the
// sole frame delimiter, so an embedded newline would desynchronise the peer.
var ErrEmbeddedNewline = errors.New("acp: frame contains embedded newline")

// frameWriter serialises outbound frames onto a single io.Writer. Concurrent
// callers may invoke writeFrame safely; writes are fully serialised so partial
// frames cannot interleave.
type frameWriter struct {
	mu  sync.Mutex
	bw  *bufio.Writer
	buf bytes.Buffer
}

func newFrameWriter(w io.Writer) *frameWriter {
	return &frameWriter{bw: bufio.NewWriter(w)}
}

// writeFrame marshals v as JSON, validates that the encoding contains no
// embedded newlines, and writes the payload followed by a single '\n'.
func (w *frameWriter) writeFrame(v any) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.buf.Reset()
	enc := json.NewEncoder(&w.buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("acp: marshal frame: %w", err)
	}
	// json.Encoder.Encode appends a trailing '\n'; drop it so we can validate
	// the payload itself and emit exactly one terminator.
	payload := bytes.TrimRight(w.buf.Bytes(), "\n")
	if bytes.ContainsRune(payload, '\n') {
		return ErrEmbeddedNewline
	}
	if _, err := w.bw.Write(payload); err != nil {
		return err
	}
	if err := w.bw.WriteByte('\n'); err != nil {
		return err
	}
	return w.bw.Flush()
}

// frameReader reads newline-delimited JSON frames. It is not safe for
// concurrent use; a single goroutine should drive the read loop.
type frameReader struct {
	sc *bufio.Scanner
}

func newFrameReader(r io.Reader) *frameReader {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), maxLineBytes)
	return &frameReader{sc: sc}
}

// readFrame returns the next frame's raw bytes (without the trailing newline),
// or io.EOF when the underlying stream closes cleanly. Blank lines are
// skipped so stray whitespace does not surface as parse errors.
func (r *frameReader) readFrame() ([]byte, error) {
	for r.sc.Scan() {
		line := r.sc.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		// sc.Bytes() is only valid until the next Scan; copy before returning.
		out := make([]byte, len(line))
		copy(out, line)
		return out, nil
	}
	if err := r.sc.Err(); err != nil {
		return nil, err
	}
	return nil, io.EOF
}
