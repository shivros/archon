package app

import "strings"

type ChatInputAddon struct {
	optionPicker *SelectPicker
	controlSpans []composeControlSpan
	optionTarget composeOptionKind
}

func NewChatInputAddon(width, pickerHeight int) *ChatInputAddon {
	return &ChatInputAddon{
		optionPicker: NewSelectPicker(width, pickerHeight),
	}
}

func (a *ChatInputAddon) SetPickerSize(width, height int) {
	if a == nil || a.optionPicker == nil {
		return
	}
	a.optionPicker.SetSize(width, height)
}

func (a *ChatInputAddon) SetControlSpans(spans []composeControlSpan) {
	if a == nil {
		return
	}
	if len(spans) == 0 {
		a.controlSpans = nil
		return
	}
	a.controlSpans = append(a.controlSpans[:0], spans...)
}

func (a *ChatInputAddon) ControlSpans() []composeControlSpan {
	if a == nil || len(a.controlSpans) == 0 {
		return nil
	}
	out := make([]composeControlSpan, len(a.controlSpans))
	copy(out, a.controlSpans)
	return out
}

func (a *ChatInputAddon) OpenOptionPicker(target composeOptionKind, options []selectOption, selectedID string, width int) bool {
	if a == nil || a.optionPicker == nil || target == composeOptionNone || len(options) == 0 {
		return false
	}
	selectedID = strings.TrimSpace(selectedID)
	a.optionPicker.SetOptions(options)
	a.optionPicker.SelectID(selectedID)
	a.optionTarget = target
	height := len(options)
	if height < 3 {
		height = 3
	}
	if height > 8 {
		height = 8
	}
	if width <= 0 {
		width = minViewportWidth
	}
	a.optionPicker.SetSize(width, height)
	return true
}

func (a *ChatInputAddon) CloseOptionPicker() {
	if a == nil {
		return
	}
	a.optionTarget = composeOptionNone
}

func (a *ChatInputAddon) OptionPickerOpen() bool {
	return a != nil && a.optionTarget != composeOptionNone
}

func (a *ChatInputAddon) OptionTarget() composeOptionKind {
	if a == nil {
		return composeOptionNone
	}
	return a.optionTarget
}

func (a *ChatInputAddon) OptionPickerMove(delta int) {
	if a == nil || a.optionPicker == nil || delta == 0 {
		return
	}
	a.optionPicker.Move(delta)
}

func (a *ChatInputAddon) OptionPickerHandleClick(row int) bool {
	if a == nil || a.optionPicker == nil {
		return false
	}
	return a.optionPicker.HandleClick(row)
}

func (a *ChatInputAddon) OptionPickerSelectedID() string {
	if a == nil || a.optionPicker == nil {
		return ""
	}
	return a.optionPicker.SelectedID()
}

func (a *ChatInputAddon) OptionPickerView() string {
	if a == nil || a.optionPicker == nil {
		return ""
	}
	return a.optionPicker.View()
}
