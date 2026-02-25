package daemon

import (
	"io"
	"os"
	"sync"
)

var _ ProviderSink = (*logSink)(nil)

type logSink struct {
	stdoutFile *os.File
	stderrFile *os.File
	stdoutBuf  *logBuffer
	stderrBuf  *logBuffer
	debug      *debugSink
	mu         sync.Mutex
}

func newLogSink(stdoutFile, stderrFile *os.File, stdoutBuf, stderrBuf *logBuffer, debug *debugSink) *logSink {
	return &logSink{
		stdoutFile: stdoutFile,
		stderrFile: stderrFile,
		stdoutBuf:  stdoutBuf,
		stderrBuf:  stderrBuf,
		debug:      debug,
	}
}

func (s *logSink) StdoutWriter() io.Writer {
	return io.MultiWriter(s.stdoutFile, &bufferWriter{buffer: s.stdoutBuf}, &debugBufferWriter{sink: s.debug, stream: "stdout"})
}

func (s *logSink) StderrWriter() io.Writer {
	return io.MultiWriter(s.stderrFile, &bufferWriter{buffer: s.stderrBuf}, &debugBufferWriter{sink: s.debug, stream: "stderr"})
}

func (s *logSink) Write(stream string, data []byte) {
	if len(data) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	switch stream {
	case "stderr":
		_, _ = s.stderrFile.Write(data)
		if s.stderrBuf != nil {
			s.stderrBuf.Append(data)
		}
	default:
		_, _ = s.stdoutFile.Write(data)
		if s.stdoutBuf != nil {
			s.stdoutBuf.Append(data)
		}
	}
}

func (s *logSink) Close() {
	if s.stdoutFile != nil {
		_ = s.stdoutFile.Close()
	}
	if s.stderrFile != nil {
		_ = s.stderrFile.Close()
	}
	if s.debug != nil {
		s.debug.Close()
	}
}

func (s *logSink) WriteDebug(stream string, data []byte) {
	if s == nil || s.debug == nil {
		return
	}
	s.debug.Write(stream, data)
}

type debugBufferWriter struct {
	sink   *debugSink
	stream string
}

func (w *debugBufferWriter) Write(p []byte) (int, error) {
	if w.sink == nil || len(p) == 0 {
		return len(p), nil
	}
	w.sink.Write(w.stream, p)
	return len(p), nil
}
