package app

import tea "charm.land/bubbletea/v2"

type groupPickerStepHandler struct {
	picker          *GroupPicker
	keys            KeyResolver
	pasteNormalizer PickerPasteNormalizer
	setStatus       func(string)
	onCancel        func()
	onConfirm       func() tea.Cmd
}

func (h groupPickerStepHandler) Update(msg tea.Msg) (bool, tea.Cmd) {
	tac := newPickerTypeAheadController(h.keys.keyString, h.keys.keyMatchesCommand, h.pasteNormalizer)
	switch msg := msg.(type) {
	case tea.PasteMsg:
		if h.picker != nil {
			tac.handlePaste(msg, h.picker)
		}
		return true, nil
	case tea.KeyMsg:
		if h.keys.keyMatchesCommand(msg, KeyCommandToggleSidebar, "ctrl+b") {
			return true, nil
		}
		switch h.keys.keyString(msg) {
		case "esc":
			if h.picker != nil && h.picker.ClearQuery() {
				h.setStatus("filter cleared")
				return true, nil
			}
			h.onCancel()
			return true, nil
		case "enter":
			return true, h.onConfirm()
		case " ", "space":
			if h.picker != nil {
				h.picker.Toggle()
			}
			return true, nil
		case "j", "down":
			if h.picker != nil {
				h.picker.Move(1)
			}
			return true, nil
		case "k", "up":
			if h.picker != nil {
				h.picker.Move(-1)
			}
			return true, nil
		}
		if h.picker != nil {
			tac.handleKey(msg, h.picker)
		}
		return true, nil
	}
	return true, nil
}
