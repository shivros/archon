package app

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
)

func (m *Model) requestTranscriptStreamOpenCmd(sessionID, afterRevision string, source TranscriptAttachmentSource, reconnectSource string) tea.Cmd {
	return m.requestTranscriptStreamOpenCmdWithContext(sessionID, afterRevision, source, reconnectSource, nil)
}

func (m *Model) requestTranscriptStreamOpenCmdWithContext(
	sessionID, afterRevision string,
	source TranscriptAttachmentSource,
	reconnectSource string,
	parent context.Context,
) tea.Cmd {
	if m == nil || m.sessionTranscriptAPI == nil {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	afterRevision = strings.TrimSpace(afterRevision)
	if sessionID == "" {
		return nil
	}
	source = normalizeTranscriptAttachmentSource(source)
	attachment := m.transcriptAttachmentCoordinatorOrDefault().Begin(sessionID, source, afterRevision)
	if reconnectSource = strings.TrimSpace(reconnectSource); reconnectSource != "" {
		m.recordReconnectAttempt(sessionID, m.providerForSessionID(sessionID), "transcript", reconnectSource)
	}
	if m.transcriptStream != nil && m.transcriptStream.HasStream() {
		m.transcriptStream.DetachStream()
	}
	m.appendTranscriptSessionTrace(
		sessionID,
		"generation_created generation=%d source=%s after_revision=%s",
		attachment.Generation,
		attachment.Source,
		attachment.AfterRevision,
	)
	return openTranscriptStreamCmdWithContextAndRequest(
		m.sessionTranscriptAPI,
		sessionID,
		afterRevision,
		parent,
		transcriptStreamOpenRequest{
			Source:     source,
			Generation: attachment.Generation,
		},
	)
}
