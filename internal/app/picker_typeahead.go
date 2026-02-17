package app

import (
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

type queryPicker interface {
	Query() string
	AppendQuery(text string) bool
	BackspaceQuery() bool
	ClearQuery() bool
}

func (m *Model) applyPickerTypeAhead(msg tea.KeyMsg, picker queryPicker) bool {
	if picker == nil {
		return false
	}
	switch m.keyString(msg) {
	case "backspace", "ctrl+h":
		return picker.BackspaceQuery()
	case "ctrl+u":
		return picker.ClearQuery()
	}
	text := pickerTypeAheadText(msg)
	if text == "" {
		return false
	}
	return picker.AppendQuery(text)
}

func (m *Model) applyPickerPaste(msg tea.PasteMsg, picker queryPicker) bool {
	if picker == nil {
		return false
	}
	text := strings.TrimSpace(msg.Content)
	if text == "" {
		return false
	}
	return picker.AppendQuery(text)
}

func pickerTypeAheadText(msg tea.KeyMsg) string {
	key := msg.Key()
	if key.Text != "" {
		return key.Text
	}
	raw := strings.TrimSpace(msg.String())
	if raw == "" || utf8.RuneCountInString(raw) != 1 {
		return ""
	}
	return raw
}
