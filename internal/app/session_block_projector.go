package app

import "context"

type SessionBlockProjectionInput struct {
	Provider    string
	Rules       sessionBlockProjectionRules
	Items       []map[string]any
	Previous    []ChatBlock
	Approvals   []*ApprovalRequest
	Resolutions []*ApprovalResolution
}

type SessionBlockProjector interface {
	ProjectSessionBlocks(context.Context, SessionBlockProjectionInput) ([]ChatBlock, error)
}

type defaultSessionBlockProjector struct{}

func WithSessionBlockProjector(projector SessionBlockProjector) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if projector == nil {
			m.sessionBlockProjector = defaultSessionBlockProjector{}
			return
		}
		m.sessionBlockProjector = projector
	}
}

func (m *Model) sessionBlockProjectorOrDefault() SessionBlockProjector {
	if m == nil || m.sessionBlockProjector == nil {
		return defaultSessionBlockProjector{}
	}
	return m.sessionBlockProjector
}

func (defaultSessionBlockProjector) ProjectSessionBlocks(ctx context.Context, input SessionBlockProjectionInput) ([]ChatBlock, error) {
	return projectSessionBlocksFromItemsWithContext(
		ctx,
		input.Provider,
		input.Rules,
		input.Items,
		input.Previous,
		input.Approvals,
		input.Resolutions,
	)
}
