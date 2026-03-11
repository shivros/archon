package app

import "strings"

type TranscriptRenderProjectionInput struct {
	SessionID    string
	Provider     string
	Blocks       []ChatBlock
	Approvals    []*ApprovalRequest
	Resolutions  []*ApprovalResolution
	Composer     TranscriptComposer
	ApplyOverlay func(sessionID string, blocks []ChatBlock) []ChatBlock
}

type TranscriptRenderProjector interface {
	Project(input TranscriptRenderProjectionInput) []ChatBlock
}

type defaultTranscriptRenderProjector struct{}

func NewDefaultTranscriptRenderProjector() TranscriptRenderProjector {
	return defaultTranscriptRenderProjector{}
}

func (defaultTranscriptRenderProjector) Project(input TranscriptRenderProjectionInput) []ChatBlock {
	blocks := append([]ChatBlock(nil), input.Blocks...)
	provider := strings.TrimSpace(input.Provider)
	if input.Composer != nil {
		blocks = input.Composer.MergeApprovals(
			blocks,
			filterApprovalRequestsForProvider(provider, input.Approvals),
			filterApprovalResolutionsForProvider(provider, input.Resolutions),
			nil,
		)
	}
	if input.ApplyOverlay != nil {
		blocks = input.ApplyOverlay(strings.TrimSpace(input.SessionID), blocks)
	}
	return blocks
}

func (m *Model) transcriptRenderProjectorOrDefault() TranscriptRenderProjector {
	if m == nil || m.transcriptRenderProjector == nil {
		return NewDefaultTranscriptRenderProjector()
	}
	return m.transcriptRenderProjector
}

func WithTranscriptRenderProjector(projector TranscriptRenderProjector) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if projector == nil {
			m.transcriptRenderProjector = NewDefaultTranscriptRenderProjector()
			return
		}
		m.transcriptRenderProjector = projector
	}
}
