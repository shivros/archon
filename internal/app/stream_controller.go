package app

import (
	"strings"

	"control/internal/types"
)

type StreamController struct {
	logEvents        <-chan types.LogEvent
	cancel           func()
	pending          string
	lines            []string
	maxLines         int
	maxEventsPerTick int
}

func NewStreamController(maxLines, maxEventsPerTick int) *StreamController {
	return &StreamController{
		maxLines:         maxLines,
		maxEventsPerTick: maxEventsPerTick,
	}
}

func (s *StreamController) Reset() {
	if s == nil {
		return
	}
	if s.cancel != nil {
		s.cancel()
	}
	s.cancel = nil
	s.logEvents = nil
	s.pending = ""
	s.lines = nil
}

func (s *StreamController) SetStream(ch <-chan types.LogEvent, cancel func()) {
	if s == nil {
		return
	}
	s.logEvents = ch
	s.cancel = cancel
}

func (s *StreamController) SetSnapshot(lines []string) {
	if s == nil {
		return
	}
	s.pending = ""
	s.lines = trimLines(lines, s.maxLines)
}

func (s *StreamController) Lines() []string {
	if s == nil {
		return nil
	}
	return s.lines
}

func (s *StreamController) ConsumeTick() (lines []string, changed bool, closed bool) {
	if s == nil || s.logEvents == nil {
		return nil, false, false
	}
	var builder strings.Builder
	drain := true
	for i := 0; i < s.maxEventsPerTick && drain; i++ {
		select {
		case event, ok := <-s.logEvents:
			if !ok {
				s.logEvents = nil
				s.cancel = nil
				closed = true
				drain = false
				break
			}
			builder.WriteString(event.Chunk)
		default:
			drain = false
		}
	}
	if builder.Len() > 0 {
		s.appendText(builder.String())
		changed = true
	}
	return s.lines, changed, closed
}

func (s *StreamController) appendText(text string) {
	combined := s.pending + text
	parts := strings.Split(combined, "\n")
	if len(parts) == 0 {
		return
	}
	if len(parts) == 1 {
		s.pending = parts[0]
		return
	}
	s.pending = parts[len(parts)-1]
	s.lines = append(s.lines, parts[:len(parts)-1]...)
	if s.maxLines > 0 && len(s.lines) > s.maxLines {
		s.lines = s.lines[len(s.lines)-s.maxLines:]
	}
}

func trimLines(lines []string, maxLines int) []string {
	if maxLines <= 0 || len(lines) <= maxLines {
		return lines
	}
	return lines[len(lines)-maxLines:]
}
