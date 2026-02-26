package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

const (
	debugMetaControlCopy     ChatMetaControlID = "debug_copy"
	debugMetaControlToggle   ChatMetaControlID = "debug_toggle"
	debugPanelLoadingMessage                   = "Loading debug stream..."
)

func (m *Model) refreshDebugPanelContent() tea.Cmd {
	if m == nil || m.debugPanel == nil {
		return nil
	}
	m.debugPanelRefreshPending = false
	if m.debugPanelExpandedByID == nil {
		m.debugPanelExpandedByID = map[string]bool{}
	}
	entries := m.loadDebugEntries()
	if len(entries) == 0 {
		m.invalidateDebugPanelProjection()
		m.applyDebugPanelEmpty()
		return nil
	}
	coordinator := m.debugPanelProjectionCoordinatorOrDefault()
	m.debugPanelLoading = true
	if len(m.debugPanelBlocks) == 0 {
		m.debugPanel.SetContent(debugPanelLoadingMessage)
	}
	presenter := m.debugPanelPresenterOrDefault()
	renderer := m.debugPanelBlocksRendererOrDefault()
	return coordinator.Schedule(DebugPanelProjectionRequest{
		Entries:      entries,
		Width:        max(1, m.debugPanelWidth),
		ExpandedByID: cloneExpandedByID(m.debugPanelExpandedByID),
		Presenter:    presenter,
		Renderer:     renderer,
	})
}

func (m *Model) loadDebugEntries() []DebugStreamEntry {
	if m == nil {
		return nil
	}
	snapshot := m.debugStreamSnapshot
	if snapshot == nil && m.debugStream != nil {
		if typed, ok := m.debugStream.(debugStreamSnapshot); ok {
			snapshot = typed
			m.debugStreamSnapshot = typed
		}
	}
	if snapshot == nil {
		return nil
	}
	return snapshot.Entries()
}

func (m *Model) presentDebugEntries(entries []DebugStreamEntry) DebugPanelPresentation {
	presenter := m.debugPanelPresenterOrDefault()
	return presenter.Present(entries, max(1, m.debugPanelWidth), DebugPanelPresentationState{ExpandedByID: m.debugPanelExpandedByID})
}

func (m *Model) debugPanelPresenterOrDefault() debugPanelPresenter {
	if m == nil {
		return NewDefaultDebugPanelPresenter(DefaultDebugPanelDisplayPolicy())
	}
	presenter := m.debugPanelPresenter
	if presenter == nil {
		presenter = NewDefaultDebugPanelPresenter(DefaultDebugPanelDisplayPolicy())
		m.debugPanelPresenter = presenter
	}
	return presenter
}

func (m *Model) applyDebugPanelEmpty() {
	if m == nil || m.debugPanel == nil {
		return
	}
	m.debugPanelLoading = false
	m.debugPanelBlocks = nil
	m.debugPanelSpans = nil
	m.debugPanelMetaByID = nil
	m.debugPanelCopyByID = map[string]string{}
	m.debugPanel.SetContent(debugPanelWaitingMessage)
}

func (m *Model) applyDebugPanelPresentation(presentation DebugPanelPresentation) {
	if m == nil || m.debugPanel == nil {
		return
	}
	m.debugPanelLoading = false
	renderer := m.debugPanelBlocksRendererOrDefault()
	rendered, spans := renderer.Render(presentation.Blocks, max(1, m.debugPanelWidth), presentation.MetaByID)
	if strings.TrimSpace(rendered) == "" {
		rendered = debugPanelWaitingMessage
	}
	m.debugPanelBlocks = presentation.Blocks
	m.debugPanelSpans = spans
	m.debugPanelMetaByID = presentation.MetaByID
	m.debugPanelCopyByID = presentation.CopyTextByID
	m.debugPanel.SetContent(rendered)
}

func (m *Model) debugPanelBlocksRendererOrDefault() debugPanelBlocksRenderer {
	if m == nil {
		return NewDefaultDebugPanelBlocksRenderer()
	}
	renderer := m.debugPanelBlocksRenderer
	if renderer == nil {
		renderer = NewDefaultDebugPanelBlocksRenderer()
		m.debugPanelBlocksRenderer = renderer
	}
	return renderer
}

func (m *Model) applyDebugPanelProjection(msg debugPanelProjectedMsg) {
	if m == nil || m.debugPanel == nil {
		return
	}
	coordinator := m.debugPanelProjectionCoordinatorOrDefault()
	if !coordinator.IsCurrent(msg.projectionSeq) {
		return
	}
	coordinator.Consume(msg.projectionSeq)
	if msg.empty {
		m.applyDebugPanelEmpty()
		return
	}
	rendered := msg.rendered
	if strings.TrimSpace(rendered) == "" {
		rendered = debugPanelWaitingMessage
	}
	m.debugPanelLoading = false
	m.debugPanelBlocks = append([]ChatBlock(nil), msg.blocks...)
	m.debugPanelSpans = append([]renderedBlockSpan(nil), msg.spans...)
	m.debugPanelMetaByID = cloneChatBlockMetaByID(msg.metaByID)
	m.debugPanelCopyByID = cloneStringMap(msg.copyByID)
	m.debugPanel.SetContent(rendered)
}

func (m *Model) invalidateDebugPanelProjection() {
	if m == nil {
		return
	}
	m.debugPanelProjectionCoordinatorOrDefault().Invalidate()
}

func projectDebugPanelCmd(
	entries []DebugStreamEntry,
	width int,
	expandedByID map[string]bool,
	presenter debugPanelPresenter,
	renderer debugPanelBlocksRenderer,
	seq int,
) tea.Cmd {
	entriesCopy := append([]DebugStreamEntry(nil), entries...)
	expandedCopy := cloneExpandedByID(expandedByID)
	if width <= 0 {
		width = 1
	}
	if presenter == nil {
		presenter = NewDefaultDebugPanelPresenter(DefaultDebugPanelDisplayPolicy())
	}
	if renderer == nil {
		renderer = NewDefaultDebugPanelBlocksRenderer()
	}
	return func() tea.Msg {
		presentation := presenter.Present(entriesCopy, width, DebugPanelPresentationState{
			ExpandedByID: expandedCopy,
		})
		if len(presentation.Blocks) == 0 {
			return debugPanelProjectedMsg{projectionSeq: seq, empty: true}
		}
		rendered, spans := renderer.Render(presentation.Blocks, width, presentation.MetaByID)
		return debugPanelProjectedMsg{
			projectionSeq: seq,
			rendered:      rendered,
			blocks:        append([]ChatBlock(nil), presentation.Blocks...),
			spans:         append([]renderedBlockSpan(nil), spans...),
			metaByID:      cloneChatBlockMetaByID(presentation.MetaByID),
			copyByID:      cloneStringMap(presentation.CopyTextByID),
		}
	}
}

func cloneExpandedByID(values map[string]bool) map[string]bool {
	if len(values) == 0 {
		return map[string]bool{}
	}
	out := make(map[string]bool, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func (m *Model) debugPanelProjectionCoordinatorOrDefault() debugPanelProjectionCoordinator {
	if m == nil {
		return NewDefaultDebugPanelProjectionCoordinator(defaultDebugPanelProjectionPolicy{}, nil)
	}
	coordinator := m.debugPanelProjectionCoordinator
	if coordinator == nil {
		coordinator = NewDefaultDebugPanelProjectionCoordinator(m.debugPanelProjectionPolicyOrDefault(), nil)
		m.debugPanelProjectionCoordinator = coordinator
	}
	return coordinator
}

func (m *Model) applyDebugPanelControl(hit debugPanelControlHit) tea.Cmd {
	if m == nil || strings.TrimSpace(hit.BlockID) == "" {
		return nil
	}
	switch hit.ControlID {
	case debugMetaControlCopy:
		return m.copyDebugPanelBlockCmd(hit.BlockID)
	case debugMetaControlToggle:
		if m.debugPanelExpandedByID == nil {
			m.debugPanelExpandedByID = map[string]bool{}
		}
		m.debugPanelExpandedByID[hit.BlockID] = !m.debugPanelExpandedByID[hit.BlockID]
		cmd := m.refreshDebugPanelContent()
		if m.debugPanelExpandedByID[hit.BlockID] {
			m.setStatusMessage("debug event expanded")
		} else {
			m.setStatusMessage("debug event collapsed")
		}
		return cmd
	default:
		return nil
	}
}

func (m *Model) copyDebugPanelBlockCmd(blockID string) tea.Cmd {
	if m == nil {
		return nil
	}
	text := ""
	if m.debugPanelCopyByID != nil {
		text = strings.TrimSpace(m.debugPanelCopyByID[blockID])
	}
	if text == "" {
		m.setCopyStatusWarning("nothing to copy")
		return nil
	}
	return m.copyWithStatusCmd(text, "debug event copied")
}
