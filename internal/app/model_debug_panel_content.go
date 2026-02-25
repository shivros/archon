package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

const (
	debugMetaControlCopy   ChatMetaControlID = "debug_copy"
	debugMetaControlToggle ChatMetaControlID = "debug_toggle"
)

func (m *Model) refreshDebugPanelContent() {
	if m == nil || m.debugPanel == nil {
		return
	}
	if m.debugPanelExpandedByID == nil {
		m.debugPanelExpandedByID = map[string]bool{}
	}
	entries := m.loadDebugEntries()
	if len(entries) == 0 {
		m.applyDebugPanelEmpty()
		return
	}
	presentation := m.presentDebugEntries(entries)
	m.applyDebugPanelPresentation(presentation)
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
	presenter := m.debugPanelPresenter
	if presenter == nil {
		presenter = NewDefaultDebugPanelPresenter(DefaultDebugPanelDisplayPolicy())
		m.debugPanelPresenter = presenter
	}
	return presenter.Present(entries, max(1, m.debugPanelWidth), DebugPanelPresentationState{ExpandedByID: m.debugPanelExpandedByID})
}

func (m *Model) applyDebugPanelEmpty() {
	if m == nil || m.debugPanel == nil {
		return
	}
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
	renderer := m.debugPanelBlocksRenderer
	if renderer == nil {
		renderer = NewDefaultDebugPanelBlocksRenderer()
		m.debugPanelBlocksRenderer = renderer
	}
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
		m.refreshDebugPanelContent()
		if m.debugPanelExpandedByID[hit.BlockID] {
			m.setStatusMessage("debug event expanded")
		} else {
			m.setStatusMessage("debug event collapsed")
		}
		return nil
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
