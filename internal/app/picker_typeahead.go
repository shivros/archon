package app

import (
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"control/internal/app/sanitizer"
)

type queryPicker interface {
	Query() string
	AppendQuery(text string) bool
	BackspaceQuery() bool
	ClearQuery() bool
}

const maxPickerPasteRunes = 256

type PickerPasteNormalizer interface {
	NormalizePickerPaste(text string) string
}

type defaultPickerPasteNormalizer struct{}

func (defaultPickerPasteNormalizer) NormalizePickerPaste(text string) string {
	text = sanitizer.RemoveEscapeSequences(text)
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return ""
	}
	normalized := strings.Join(parts, " ")
	runes := []rune(normalized)
	if len(runes) > maxPickerPasteRunes {
		normalized = string(runes[:maxPickerPasteRunes])
	}
	return normalized
}

func WithPickerPasteNormalizer(normalizer PickerPasteNormalizer) ModelOption {
	return func(m *Model) {
		if m == nil || normalizer == nil {
			return
		}
		m.pickerPasteNormalizer = normalizer
	}
}

type pickerTypeAheadController struct {
	keyString       func(tea.KeyMsg) string
	pasteNormalizer PickerPasteNormalizer
}

func newPickerTypeAheadController(keyString func(tea.KeyMsg) string, pasteNormalizer PickerPasteNormalizer) pickerTypeAheadController {
	if pasteNormalizer == nil {
		pasteNormalizer = defaultPickerPasteNormalizer{}
	}
	return pickerTypeAheadController{
		keyString:       keyString,
		pasteNormalizer: pasteNormalizer,
	}
}

func (c pickerTypeAheadController) Handle(msg tea.Msg, picker queryPicker) bool {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return c.handleKey(msg, picker)
	case tea.PasteMsg:
		return c.handlePaste(msg, picker)
	default:
		return false
	}
}

func (c pickerTypeAheadController) handleKey(msg tea.KeyMsg, picker queryPicker) bool {
	if picker == nil {
		return false
	}
	key := msg.String()
	if c.keyString != nil {
		key = c.keyString(msg)
	}
	switch key {
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

func (c pickerTypeAheadController) handlePaste(msg tea.PasteMsg, picker queryPicker) bool {
	if picker == nil {
		return false
	}
	normalizer := c.pasteNormalizer
	if normalizer == nil {
		normalizer = defaultPickerPasteNormalizer{}
	}
	text := normalizer.NormalizePickerPaste(msg.Content)
	if text == "" {
		return false
	}
	return picker.AppendQuery(text)
}

func (m *Model) pickerTypeAheadController() pickerTypeAheadController {
	var keyFn func(tea.KeyMsg) string
	var normalizer PickerPasteNormalizer
	if m != nil {
		keyFn = m.keyString
		normalizer = m.pickerPasteNormalizer
	}
	return newPickerTypeAheadController(keyFn, normalizer)
}

func (m *Model) applyPickerTypeAhead(msg tea.KeyMsg, picker queryPicker) bool {
	return m.pickerTypeAheadController().handleKey(msg, picker)
}

func (m *Model) applyPickerPaste(msg tea.PasteMsg, picker queryPicker) bool {
	return m.pickerTypeAheadController().handlePaste(msg, picker)
}

type composeOptionQueryPicker struct {
	model *Model
}

func (p composeOptionQueryPicker) Query() string {
	if p.model == nil {
		return ""
	}
	return p.model.composeOptionPickerQuery()
}

func (p composeOptionQueryPicker) AppendQuery(text string) bool {
	if p.model == nil {
		return false
	}
	return p.model.composeOptionPickerAppendQuery(text)
}

func (p composeOptionQueryPicker) BackspaceQuery() bool {
	if p.model == nil {
		return false
	}
	return p.model.composeOptionPickerBackspaceQuery()
}

func (p composeOptionQueryPicker) ClearQuery() bool {
	if p.model == nil {
		return false
	}
	return p.model.composeOptionPickerClearQuery()
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
