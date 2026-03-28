package app

import (
	"errors"
	"strings"
	"time"

	"control/internal/apicode"
	"control/internal/client"

	tea "charm.land/bubbletea/v2"
)

const (
	transcriptHistoryPendingRetryDelay = 500 * time.Millisecond
	transcriptHistoryPendingRetryLimit = 2
)

func transcriptSnapshotResponseKey(msg transcriptSnapshotMsg) string {
	responseKey := strings.TrimSpace(msg.key)
	if responseKey == "" && strings.TrimSpace(msg.id) != "" {
		responseKey = "sess:" + strings.TrimSpace(msg.id)
	}
	return responseKey
}

func (m *Model) handleTranscriptSnapshotError(msg transcriptSnapshotMsg, source TranscriptAttachmentSource, responseKey string) (handled bool, cmd tea.Cmd) {
	if msg.err == nil {
		return false, nil
	}
	m.recordHistoryWindowFromResponse(responseKey, msg.requestedLines, 0, msg.err)
	if isCanceledRequestError(msg.err) {
		return true, nil
	}
	if m.shouldDropTranscriptSnapshotByKey(msg) {
		return true, nil
	}
	if isTranscriptHistoryPendingError(msg.err) {
		return true, m.handleTranscriptSnapshotPending(msg, source, responseKey)
	}
	m.setBackgroundError("transcript snapshot error: " + msg.err.Error())
	if msg.key != "" && msg.key == m.loadingKey {
		m.setContentText("Error loading transcript.")
		m.clearSessionLoadingState()
	}
	return true, m.maybeOpenTranscriptFollowAfterSnapshot(msg.id, source, "")
}

func isTranscriptHistoryPendingError(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *client.APIError
	if !errors.As(err, &apiErr) || apiErr == nil {
		return false
	}
	return strings.TrimSpace(apiErr.Code) == apicode.ErrorCodeTranscriptHistoryPending
}

func (m *Model) handleTranscriptSnapshotPending(msg transcriptSnapshotMsg, source TranscriptAttachmentSource, responseKey string) tea.Cmd {
	if m == nil {
		return nil
	}
	m.setBackgroundStatus("transcript history pending; retrying")
	cmds := make([]tea.Cmd, 0, 2)
	if cmd := m.maybeOpenTranscriptFollowAfterSnapshot(msg.id, source, ""); cmd != nil {
		cmds = append(cmds, cmd)
	}
	if cmd := m.maybeRetryPendingTranscriptSnapshot(msg, source, responseKey); cmd != nil {
		cmds = append(cmds, cmd)
	}
	if len(cmds) == 0 {
		m.settlePendingTranscriptSnapshotLoading(msg)
		return nil
	}
	return tea.Batch(cmds...)
}

func (m *Model) settlePendingTranscriptSnapshotLoading(msg transcriptSnapshotMsg) {
	if m == nil {
		return
	}
	if key := strings.TrimSpace(msg.key); key != "" && key == strings.TrimSpace(m.loadingKey) {
		m.finishSessionLoadLatencyForKey(key, uiLatencyOutcomeError)
		m.clearSessionLoadingState()
		return
	}
	m.markTranscriptLoadingSignalWithOutcome(msg.id, uiLatencyOutcomeError)
}

func (m *Model) maybeRetryPendingTranscriptSnapshot(msg transcriptSnapshotMsg, source TranscriptAttachmentSource, responseKey string) tea.Cmd {
	if m == nil || m.sessionTranscriptAPI == nil {
		return nil
	}
	responseKey = strings.TrimSpace(responseKey)
	if responseKey == "" {
		return nil
	}
	if m.pendingTranscriptSnapshotRetryCount == nil {
		m.pendingTranscriptSnapshotRetryCount = map[string]int{}
	}
	attempts := m.pendingTranscriptSnapshotRetryCount[responseKey]
	if attempts >= transcriptHistoryPendingRetryLimit {
		return nil
	}
	m.pendingTranscriptSnapshotRetryCount[responseKey] = attempts + 1
	return retryTranscriptSnapshotCmdWithDelay(
		m.sessionTranscriptAPI,
		msg.id,
		msg.key,
		msg.requestedLines,
		m.requestScopeContext(requestScopeSessionLoad),
		transcriptHistoryPendingRetryDelay,
		transcriptSnapshotRequest{
			Source:        source,
			Authoritative: msg.authoritative,
		},
	)
}

func (m *Model) shouldDropTranscriptSnapshotByKey(msg transcriptSnapshotMsg) bool {
	return strings.TrimSpace(msg.key) != "" && strings.TrimSpace(msg.key) != strings.TrimSpace(m.pendingSessionKey)
}

func (m *Model) applyTranscriptSnapshotPayload(msg transcriptSnapshotMsg, source TranscriptAttachmentSource) (blocks []ChatBlock, authoritative bool, applied bool) {
	if msg.snapshot == nil {
		return nil, false, false
	}
	applied = true
	authoritative = msg.authoritative || m.transcriptRecoveryCoordinatorOrDefault().ShouldApplyAuthoritativeSnapshot(msg.id)
	if m.transcriptStream != nil {
		if authoritative {
			_, applied = m.transcriptStream.SetAuthoritativeSnapshot(*msg.snapshot)
		} else {
			_, applied = m.transcriptStream.SetSnapshot(*msg.snapshot)
		}
	}
	if !applied {
		m.recordTranscriptBoundaryMetric(newStaleRevisionDropMetric(
			transcriptReasonSnapshotSuperseded,
			transcriptSourceSessionBlocksProject,
			msg.id,
			m.providerForSessionID(msg.id),
		))
		m.appendTranscriptSessionTrace(
			msg.id,
			"snapshot_rejected source=%s reason=%s revision=%s",
			source,
			transcriptReasonSnapshotSuperseded,
			msg.snapshot.Revision,
		)
		return nil, authoritative, false
	}
	if authoritative {
		m.transcriptRecoveryCoordinatorOrDefault().MarkRecovered(msg.id)
	}
	blocks = renderableTranscriptBlocksToChatBlocks(msg.snapshot.Blocks)
	if m.transcriptStream != nil {
		blocks = m.transcriptStream.Blocks()
	}
	return blocks, authoritative, true
}

func (m *Model) projectTranscriptSnapshotBlocks(sessionID string, snapshotBlocks []ChatBlock) []ChatBlock {
	blocks := append([]ChatBlock(nil), snapshotBlocks...)
	return m.transcriptRenderProjectorOrDefault().Project(TranscriptRenderProjectionInput{
		SessionID:   sessionID,
		Provider:    m.providerForSessionID(sessionID),
		Blocks:      blocks,
		Approvals:   m.sessionApprovals[sessionID],
		Resolutions: m.sessionApprovalResolutions[sessionID],
		Composer:    m.transcriptComposerOrDefault(),
		ApplyOverlay: func(_ string, next []ChatBlock) []ChatBlock {
			return next
		},
	})
}

func (m *Model) transcriptSnapshotFollowUps(msg transcriptSnapshotMsg, source TranscriptAttachmentSource, responseKey string, blocks []ChatBlock) tea.Cmd {
	if m != nil && responseKey != "" && m.pendingTranscriptSnapshotRetryCount != nil {
		delete(m.pendingTranscriptSnapshotRetryCount, responseKey)
	}
	cmds := make([]tea.Cmd, 0, 2)
	if cmd := m.maybeBackfillSnapshotMissingUserTurns(msg.id, responseKey, blocks); cmd != nil {
		cmds = append(cmds, cmd)
	}
	revision := ""
	if msg.snapshot != nil {
		revision = strings.TrimSpace(msg.snapshot.Revision.String())
	}
	if cmd := m.maybeOpenTranscriptFollowAfterSnapshot(msg.id, source, revision); cmd != nil {
		cmds = append(cmds, cmd)
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}
