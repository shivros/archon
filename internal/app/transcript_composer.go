package app

type TranscriptComposer interface {
	AppendOptimisticUser(base []ChatBlock, text string) ([]ChatBlock, int)
	MarkUserStatus(base []ChatBlock, headerIndex int, status ChatStatus) ([]ChatBlock, bool)
	MergeApprovals(base []ChatBlock, requests []*ApprovalRequest, resolutions []*ApprovalResolution, previous []ChatBlock) []ChatBlock
}

type defaultTranscriptComposer struct{}

func NewDefaultTranscriptComposer() TranscriptComposer {
	return defaultTranscriptComposer{}
}

func (defaultTranscriptComposer) AppendOptimisticUser(base []ChatBlock, text string) ([]ChatBlock, int) {
	tp := NewChatTranscript(0)
	tp.SetBlocks(base)
	headerIndex := tp.AppendUserMessage(text)
	if headerIndex >= 0 {
		_ = tp.MarkUserMessageSending(headerIndex)
	}
	return append([]ChatBlock(nil), tp.Blocks()...), headerIndex
}

func (defaultTranscriptComposer) MarkUserStatus(base []ChatBlock, headerIndex int, status ChatStatus) ([]ChatBlock, bool) {
	tp := NewChatTranscript(0)
	tp.SetBlocks(base)
	changed := false
	switch status {
	case ChatStatusFailed:
		changed = tp.MarkUserMessageFailed(headerIndex)
	case ChatStatusSending:
		changed = tp.MarkUserMessageSending(headerIndex)
	default:
		changed = tp.MarkUserMessageSent(headerIndex)
	}
	if !changed {
		return append([]ChatBlock(nil), base...), false
	}
	return append([]ChatBlock(nil), tp.Blocks()...), true
}

func (defaultTranscriptComposer) MergeApprovals(base []ChatBlock, requests []*ApprovalRequest, resolutions []*ApprovalResolution, previous []ChatBlock) []ChatBlock {
	blocks := mergeApprovalBlocks(base, requests, resolutions)
	if len(previous) > 0 {
		blocks = preserveApprovalPositions(previous, blocks)
	}
	return blocks
}

func WithTranscriptComposer(composer TranscriptComposer) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		m.transcriptComposer = composer
	}
}

func (m *Model) transcriptComposerOrDefault() TranscriptComposer {
	if m == nil || m.transcriptComposer == nil {
		return NewDefaultTranscriptComposer()
	}
	return m.transcriptComposer
}
