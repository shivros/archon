package logging

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Level int

const (
	Debug Level = iota
	Info
	Warn
	Error
)

type Field struct {
	Key   string
	Value any
}

type Logger interface {
	Debug(msg string, fields ...Field)
	Info(msg string, fields ...Field)
	Warn(msg string, fields ...Field)
	Error(msg string, fields ...Field)
	With(fields ...Field) Logger
	Enabled(level Level) bool
}

type logfmtLogger struct {
	out    io.Writer
	level  Level
	fields []Field
	mu     *sync.Mutex
}

func New(out io.Writer, level Level) Logger {
	if out == nil {
		out = os.Stdout
	}
	return &logfmtLogger{out: out, level: level, mu: &sync.Mutex{}}
}

func Nop() Logger {
	return &logfmtLogger{out: io.Discard, level: Error, mu: &sync.Mutex{}}
}

func (l *logfmtLogger) Enabled(level Level) bool {
	if l == nil {
		return false
	}
	return level >= l.level
}

func (l *logfmtLogger) With(fields ...Field) Logger {
	if l == nil {
		return Nop()
	}
	next := &logfmtLogger{
		out:    l.out,
		level:  l.level,
		fields: append(append([]Field{}, l.fields...), fields...),
		mu:     l.mu,
	}
	return next
}

func (l *logfmtLogger) Debug(msg string, fields ...Field) { l.log(Debug, msg, fields...) }
func (l *logfmtLogger) Info(msg string, fields ...Field)  { l.log(Info, msg, fields...) }
func (l *logfmtLogger) Warn(msg string, fields ...Field)  { l.log(Warn, msg, fields...) }
func (l *logfmtLogger) Error(msg string, fields ...Field) { l.log(Error, msg, fields...) }

func (l *logfmtLogger) log(level Level, msg string, fields ...Field) {
	if l == nil || level < l.level {
		return
	}
	all := make([]Field, 0, len(l.fields)+len(fields)+3)
	all = append(all, Field{Key: "ts", Value: time.Now().UTC().Format(time.RFC3339Nano)})
	all = append(all, Field{Key: "level", Value: levelString(level)})
	all = append(all, Field{Key: "msg", Value: msg})
	all = append(all, l.fields...)
	all = append(all, fields...)

	l.mu.Lock()
	defer l.mu.Unlock()
	for i, field := range all {
		if i > 0 {
			_, _ = io.WriteString(l.out, " ")
		}
		_, _ = io.WriteString(l.out, field.Key)
		_, _ = io.WriteString(l.out, "=")
		_, _ = io.WriteString(l.out, formatValue(field.Value))
	}
	_, _ = io.WriteString(l.out, "\n")
}

func formatValue(value any) string {
	switch v := value.(type) {
	case nil:
		return "null"
	case string:
		return quoteIfNeeded(v)
	case []byte:
		return quoteIfNeeded(string(v))
	case fmt.Stringer:
		return quoteIfNeeded(v.String())
	case time.Duration:
		return quoteIfNeeded(v.String())
	case bool:
		return strconv.FormatBool(v)
	case int, int64, int32, uint, uint64, uint32, float64, float32:
		return fmt.Sprintf("%v", v)
	default:
		return quoteIfNeeded(fmt.Sprintf("%v", v))
	}
}

func quoteIfNeeded(value string) string {
	if value == "" {
		return `""`
	}
	if strings.ContainsAny(value, " \t\n\r\"=") {
		return strconv.Quote(value)
	}
	return value
}

func levelString(level Level) string {
	switch level {
	case Debug:
		return "debug"
	case Info:
		return "info"
	case Warn:
		return "warn"
	case Error:
		return "error"
	default:
		return "info"
	}
}

func ParseLevel(raw string) Level {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug":
		return Debug
	case "warn", "warning":
		return Warn
	case "error":
		return Error
	default:
		return Info
	}
}

func NewRequestID() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(buf[:])
}

func F(key string, value any) Field {
	return Field{Key: key, Value: value}
}
