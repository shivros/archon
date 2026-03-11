package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"
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
	m.setBackgroundError("transcript snapshot error: " + msg.err.Error())
	if msg.key != "" && msg.key == m.loadingKey {
		m.setContentText("Error loading transcript.")
		m.loading = false
		m.loadingKey = ""
	}
	return true, m.maybeOpenTranscriptFollowAfterSnapshot(msg.id, source, "")
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
	blocks = transcriptBlocksToChatBlocks(msg.snapshot.Blocks)
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
