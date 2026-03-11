package app

import (
	"fmt"
	"strings"
	"time"
)

const transcriptTraceMaxEntriesPerSession = 64

type transcriptSessionTraceEntry struct {
	At      time.Time
	Session string
	Event   string
}

func (m *Model) appendTranscriptSessionTrace(sessionID, format string, args ...any) {
	if m == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	event := strings.TrimSpace(fmt.Sprintf(format, args...))
	if event == "" {
		return
	}
	if m.transcriptSessionTraces == nil {
		m.transcriptSessionTraces = map[string][]transcriptSessionTraceEntry{}
	}
	entries := m.transcriptSessionTraces[sessionID]
	entries = append(entries, transcriptSessionTraceEntry{
		At:      time.Now().UTC(),
		Session: sessionID,
		Event:   event,
	})
	if len(entries) > transcriptTraceMaxEntriesPerSession {
		entries = append([]transcriptSessionTraceEntry(nil), entries[len(entries)-transcriptTraceMaxEntriesPerSession:]...)
	}
	m.transcriptSessionTraces[sessionID] = entries
}

func (m *Model) transcriptSessionTrace(sessionID string) []transcriptSessionTraceEntry {
	if m == nil || m.transcriptSessionTraces == nil {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	entries := m.transcriptSessionTraces[sessionID]
	return append([]transcriptSessionTraceEntry(nil), entries...)
}
