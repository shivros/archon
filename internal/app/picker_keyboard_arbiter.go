package app

import tea "charm.land/bubbletea/v2"

type pickerKeyboardArbiter struct {
	keyString func(tea.KeyMsg) string
	typeAhead pickerTypeAheadController
}

type pickerKeyboardHooks struct {
	PreHandleKey func(key string, msg tea.KeyMsg) (bool, tea.Cmd)
	Cancel       func() tea.Cmd
	Confirm      func() tea.Cmd
	Toggle       func()
	MoveUp       func()
	MoveDown     func()
	PageUp       func()
	PageDown     func()
	Home         func()
	End          func()
	OnTypeAhead  func()
}

func newPickerKeyboardArbiter(
	keyString func(tea.KeyMsg) string,
	keyMatchesCommand func(tea.KeyMsg, string, string) bool,
	pasteNormalizer PickerPasteNormalizer,
) pickerKeyboardArbiter {
	return pickerKeyboardArbiter{
		keyString: keyString,
		typeAhead: newPickerTypeAheadController(keyString, keyMatchesCommand, pasteNormalizer),
	}
}

func (a pickerKeyboardArbiter) Handle(msg tea.Msg, picker queryPicker, hooks pickerKeyboardHooks) (bool, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.PasteMsg:
		if picker != nil && a.typeAhead.handlePaste(msg, picker) {
			if hooks.OnTypeAhead != nil {
				hooks.OnTypeAhead()
			}
			return true, nil
		}
		return false, nil
	case tea.KeyMsg:
		key := msg.String()
		if a.keyString != nil {
			key = a.keyString(msg)
		}
		if hooks.PreHandleKey != nil {
			if handled, cmd := hooks.PreHandleKey(key, msg); handled {
				return true, cmd
			}
		}
		switch key {
		case "esc":
			if hooks.Cancel != nil {
				return true, hooks.Cancel()
			}
		case "enter":
			if hooks.Confirm != nil {
				return true, hooks.Confirm()
			}
		case " ", "space":
			if hooks.Toggle != nil {
				hooks.Toggle()
				return true, nil
			}
		case "up":
			if hooks.MoveUp != nil {
				hooks.MoveUp()
				return true, nil
			}
		case "down":
			if hooks.MoveDown != nil {
				hooks.MoveDown()
				return true, nil
			}
		case "pgup":
			if hooks.PageUp != nil {
				hooks.PageUp()
				return true, nil
			}
		case "pgdown":
			if hooks.PageDown != nil {
				hooks.PageDown()
				return true, nil
			}
		case "home":
			if hooks.Home != nil {
				hooks.Home()
				return true, nil
			}
		case "end":
			if hooks.End != nil {
				hooks.End()
				return true, nil
			}
		}
		if picker != nil && a.typeAhead.handleKey(msg, picker) {
			if hooks.OnTypeAhead != nil {
				hooks.OnTypeAhead()
			}
			return true, nil
		}
		return false, nil
	default:
		return false, nil
	}
}
