package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"control/internal/types"
)

type ChatInputAddonController struct {
	addon *ChatInputAddon
}

func NewChatInputAddonController(addon *ChatInputAddon) *ChatInputAddonController {
	return &ChatInputAddonController{addon: addon}
}

func (c *ChatInputAddonController) setPickerSize(width, height int) {
	if c == nil || c.addon == nil {
		return
	}
	c.addon.SetPickerSize(width, height)
}

func (c *ChatInputAddonController) composeControlsLine(m *Model) string {
	if c == nil || m == nil {
		return ""
	}
	if m.mode != uiModeCompose {
		if c.addon != nil {
			c.addon.SetControlSpans(nil)
		}
		return ""
	}
	options := m.composeRuntimeOptions()
	if options == nil {
		if c.addon != nil {
			c.addon.SetControlSpans(nil)
		}
		return ""
	}
	provider := m.composeProvider()
	catalog := m.providerOptionCatalog(provider)
	model := strings.TrimSpace(options.Model)
	if model == "" {
		model = "default"
	}
	reasoning := string(options.Reasoning)
	if reasoning == "" {
		reasoning = "default"
	}
	access := string(options.Access)
	if access == "" {
		access = "default"
	}
	parts := []struct {
		kind composeOptionKind
		text string
	}{
		{kind: composeOptionModel, text: "Model: " + model},
		{kind: composeOptionAccess, text: "Access: " + access},
	}
	if catalog != nil && len(m.modelReasoningLevels(provider, options.Model)) > 0 {
		parts = []struct {
			kind composeOptionKind
			text string
		}{
			{kind: composeOptionModel, text: "Model: " + model},
			{kind: composeOptionReasoning, text: "Reasoning: " + reasoning},
			{kind: composeOptionAccess, text: "Access: " + access},
		}
	}
	spans := make([]composeControlSpan, 0, len(parts))
	var b strings.Builder
	col := 0
	for i, part := range parts {
		if i > 0 {
			b.WriteString("  |  ")
			col += 5
		}
		label := part.text
		if c.addon != nil && c.addon.OptionTarget() == part.kind {
			label = "[" + label + "]"
		}
		start := col
		b.WriteString(label)
		col += len(label)
		spans = append(spans, composeControlSpan{
			kind:  part.kind,
			start: start,
			end:   col - 1,
		})
	}
	if c.addon != nil {
		c.addon.SetControlSpans(spans)
	}
	return b.String()
}

func (c *ChatInputAddonController) openComposeOptionPicker(m *Model, target composeOptionKind) bool {
	if c == nil || m == nil || c.addon == nil || target == composeOptionNone {
		return false
	}
	provider := m.composeProvider()
	if provider == "" {
		return false
	}
	catalog := m.providerOptionCatalog(provider)
	if catalog == nil {
		return false
	}
	options := make([]selectOption, 0, 8)
	selectedID := ""
	current := m.composeRuntimeOptions()
	switch target {
	case composeOptionModel:
		for _, model := range catalog.Models {
			value := strings.TrimSpace(model)
			if value == "" {
				continue
			}
			options = append(options, selectOption{id: value, label: value})
		}
		if current != nil {
			selectedID = strings.TrimSpace(current.Model)
		}
	case composeOptionReasoning:
		model := ""
		if current != nil {
			model = current.Model
		}
		for _, level := range m.modelReasoningLevels(provider, model) {
			value := strings.TrimSpace(string(level))
			if value == "" {
				continue
			}
			options = append(options, selectOption{id: value, label: value})
		}
		if current != nil {
			selectedID = strings.TrimSpace(string(current.Reasoning))
		}
	case composeOptionAccess:
		for _, level := range catalog.AccessLevels {
			value := strings.TrimSpace(string(level))
			if value == "" {
				continue
			}
			options = append(options, selectOption{id: value, label: value})
		}
		if current != nil {
			selectedID = strings.TrimSpace(string(current.Access))
		}
	}
	if len(options) == 0 {
		return false
	}
	return c.addon.OpenOptionPicker(target, options, selectedID, m.viewport.Width())
}

func (c *ChatInputAddonController) closeComposeOptionPicker() {
	if c == nil || c.addon == nil {
		return
	}
	c.addon.CloseOptionPicker()
}

func (c *ChatInputAddonController) composeOptionPickerOpen() bool {
	return c != nil && c.addon != nil && c.addon.OptionPickerOpen()
}

func (c *ChatInputAddonController) composeOptionPickerSelectedID() string {
	if c == nil || c.addon == nil {
		return ""
	}
	return c.addon.OptionPickerSelectedID()
}

func (c *ChatInputAddonController) moveComposeOptionPicker(delta int) {
	if c == nil || c.addon == nil {
		return
	}
	c.addon.OptionPickerMove(delta)
}

func (c *ChatInputAddonController) composeOptionPickerHandleClick(row int) bool {
	if c == nil || c.addon == nil {
		return false
	}
	return c.addon.OptionPickerHandleClick(row)
}

func (c *ChatInputAddonController) composeControlSpans() []composeControlSpan {
	if c == nil || c.addon == nil {
		return nil
	}
	return c.addon.ControlSpans()
}

func (c *ChatInputAddonController) applyComposeOptionSelection(m *Model, value string) tea.Cmd {
	if c == nil || c.addon == nil || m == nil {
		return nil
	}
	target := c.addon.OptionTarget()
	if target == composeOptionNone {
		return nil
	}
	options := m.composeRuntimeOptions()
	if options == nil {
		return nil
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	switch target {
	case composeOptionModel:
		options.Model = value
		m.normalizeComposeRuntimeOptionsForModel(m.composeProvider(), options)
	case composeOptionReasoning:
		options.Reasoning = types.ReasoningLevel(value)
	case composeOptionAccess:
		options.Access = types.AccessLevel(value)
	}
	provider := m.composeProvider()
	m.setComposeDefaultForProvider(provider, options)
	saveDefaults := m.saveAppStateCmd()
	if m.newSession != nil {
		m.newSession.runtimeOptions = types.CloneRuntimeOptions(options)
		m.setStatusMessage("session options updated")
		return saveDefaults
	}
	sessionID := strings.TrimSpace(m.composeSessionID())
	if sessionID == "" {
		if saveDefaults != nil {
			return saveDefaults
		}
		return nil
	}
	m.setSessionRuntimeOptionsLocal(sessionID, options)
	update := updateSessionRuntimeCmd(m.sessionAPI, sessionID, options)
	m.setStatusMessage("updating session options")
	if saveDefaults != nil {
		return tea.Batch(update, saveDefaults)
	}
	return update
}

func (c *ChatInputAddonController) composeOptionPopupView(m *Model) (string, int) {
	if c == nil || c.addon == nil || m == nil || !c.addon.OptionPickerOpen() {
		return "", 0
	}
	view := c.addon.OptionPickerView()
	if strings.TrimSpace(view) == "" {
		return "", 0
	}
	if leftPad := m.sidebarWidth(); leftPad > 0 {
		prefix := strings.Repeat(" ", leftPad+1)
		lines := strings.Split(view, "\n")
		for i := range lines {
			lines[i] = prefix + lines[i]
		}
		view = strings.Join(lines, "\n")
	}
	height := len(strings.Split(view, "\n"))
	row := m.composeControlsRow() - height
	if row < 1 {
		row = 1
	}
	return view, row
}
