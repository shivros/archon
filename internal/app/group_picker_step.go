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
	arbiter := newPickerKeyboardArbiter(h.keys.keyString, h.keys.keyMatchesCommand, h.pasteNormalizer)
	handled, cmd := arbiter.Handle(msg, h.picker, pickerKeyboardHooks{
		PreHandleKey: func(_ string, msg tea.KeyMsg) (bool, tea.Cmd) {
			if h.keys.keyMatchesCommand(msg, KeyCommandToggleSidebar, "ctrl+b") {
				return true, nil
			}
			return false, nil
		},
		Cancel: func() tea.Cmd {
			if h.picker != nil && h.picker.ClearQuery() {
				h.setStatus("filter cleared")
				return nil
			}
			h.onCancel()
			return nil
		},
		Confirm: h.onConfirm,
		Toggle: func() {
			if h.picker != nil {
				h.picker.Toggle()
			}
		},
		MoveDown: func() {
			if h.picker != nil {
				h.picker.Move(1)
			}
		},
		MoveUp: func() {
			if h.picker != nil {
				h.picker.Move(-1)
			}
		},
	})
	if handled {
		return true, cmd
	}
	return true, nil
}
