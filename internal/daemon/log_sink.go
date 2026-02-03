package daemon

import (
	"io"
	"os"
	"sync"
)

type logSink struct {
	stdoutFile *os.File
	stderrFile *os.File
	stdoutBuf  *logBuffer
	stderrBuf  *logBuffer
	mu         sync.Mutex
}

func newLogSink(stdoutFile, stderrFile *os.File, stdoutBuf, stderrBuf *logBuffer) *logSink {
	return &logSink{
		stdoutFile: stdoutFile,
		stderrFile: stderrFile,
		stdoutBuf:  stdoutBuf,
		stderrBuf:  stderrBuf,
	}
}

func (s *logSink) StdoutWriter() io.Writer {
	return io.MultiWriter(s.stdoutFile, &bufferWriter{buffer: s.stdoutBuf})
}

func (s *logSink) StderrWriter() io.Writer {
	return io.MultiWriter(s.stderrFile, &bufferWriter{buffer: s.stderrBuf})
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
}
