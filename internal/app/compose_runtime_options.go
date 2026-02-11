package app

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"control/internal/types"
)

func (m *Model) composeProvider() string {
	if m.newSession != nil {
		return strings.TrimSpace(m.newSession.provider)
	}
	sessionID := strings.TrimSpace(m.composeSessionID())
	if sessionID != "" {
		return strings.TrimSpace(m.providerForSessionID(sessionID))
	}
	return strings.TrimSpace(m.selectedSessionProvider())
}

func (m *Model) providerOptionCatalog(provider string) *types.ProviderOptionCatalog {
	name := strings.ToLower(strings.TrimSpace(provider))
	if name == "" {
		return nil
	}
	if m.providerOptions != nil {
		if options, ok := m.providerOptions[name]; ok && options != nil {
			return options
		}
	}
	if name == "codex" {
		return &types.ProviderOptionCatalog{
			Provider: "codex",
			Models: []string{
				"gpt-5.1-codex",
				"gpt-5.2-codex",
				"gpt-5.3-codex",
				"gpt-5.1-codex-max",
			},
			ReasoningLevels: []types.ReasoningLevel{
				types.ReasoningLow,
				types.ReasoningMedium,
				types.ReasoningHigh,
				types.ReasoningExtraHigh,
			},
			AccessLevels: []types.AccessLevel{
				types.AccessReadOnly,
				types.AccessOnRequest,
				types.AccessFull,
			},
			Defaults: types.SessionRuntimeOptions{
				Model:     "gpt-5.1-codex",
				Reasoning: types.ReasoningMedium,
				Access:    types.AccessOnRequest,
				Version:   1,
			},
		}
	}
	if name == "claude" {
		return &types.ProviderOptionCatalog{
			Provider: "claude",
			Models: []string{
				"sonnet",
				"opus",
			},
			AccessLevels: []types.AccessLevel{
				types.AccessReadOnly,
				types.AccessOnRequest,
				types.AccessFull,
			},
			Defaults: types.SessionRuntimeOptions{
				Model:   "sonnet",
				Access:  types.AccessOnRequest,
				Version: 1,
			},
		}
	}
	return nil
}

func (m *Model) composeRuntimeOptions() *types.SessionRuntimeOptions {
	provider := m.composeProvider()
	if provider == "" {
		return nil
	}
	var out *types.SessionRuntimeOptions
	if catalog := m.providerOptionCatalog(provider); catalog != nil {
		out = types.MergeRuntimeOptions(out, &catalog.Defaults)
	}
	if m.newSession != nil {
		if defaults := m.composeDefaultsForProvider(provider); defaults != nil {
			out = types.MergeRuntimeOptions(out, defaults)
		}
		out = types.MergeRuntimeOptions(out, m.newSession.runtimeOptions)
	} else if sessionID := strings.TrimSpace(m.composeSessionID()); sessionID != "" {
		meta := m.sessionMeta[sessionID]
		if meta != nil {
			out = types.MergeRuntimeOptions(out, meta.RuntimeOptions)
		}
	}
	if out == nil {
		out = &types.SessionRuntimeOptions{}
	}
	m.normalizeComposeRuntimeOptionsForModel(provider, out)
	return out
}

func (m *Model) composeDefaultsForProvider(provider string) *types.SessionRuntimeOptions {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" || m.appState.ComposeDefaultsByProvider == nil {
		return nil
	}
	return types.CloneRuntimeOptions(m.appState.ComposeDefaultsByProvider[provider])
}

func (m *Model) setComposeDefaultForProvider(provider string, runtimeOptions *types.SessionRuntimeOptions) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" || runtimeOptions == nil {
		return
	}
	if m.appState.ComposeDefaultsByProvider == nil {
		m.appState.ComposeDefaultsByProvider = map[string]*types.SessionRuntimeOptions{}
	}
	m.appState.ComposeDefaultsByProvider[provider] = types.CloneRuntimeOptions(runtimeOptions)
	m.hasAppState = true
}

func (m *Model) setSessionRuntimeOptionsLocal(sessionID string, runtimeOptions *types.SessionRuntimeOptions) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || runtimeOptions == nil {
		return
	}
	if m.sessionMeta == nil {
		m.sessionMeta = map[string]*types.SessionMeta{}
	}
	meta := m.sessionMeta[sessionID]
	if meta == nil {
		meta = &types.SessionMeta{SessionID: sessionID}
		m.sessionMeta[sessionID] = meta
	}
	meta.RuntimeOptions = types.CloneRuntimeOptions(runtimeOptions)
}

func (m *Model) composeControlsLine() string {
	if m.mode != uiModeCompose {
		m.composeControlSpans = nil
		return ""
	}
	options := m.composeRuntimeOptions()
	if options == nil {
		m.composeControlSpans = nil
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
		if m.composeOptionTarget == part.kind {
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
	m.composeControlSpans = spans
	return b.String()
}

func (m *Model) modelReasoningLevels(provider, model string) []types.ReasoningLevel {
	catalog := m.providerOptionCatalog(provider)
	if catalog == nil {
		return nil
	}
	model = strings.TrimSpace(model)
	if model != "" && len(catalog.ModelReasoningLevels) > 0 {
		for key, levels := range catalog.ModelReasoningLevels {
			if strings.EqualFold(strings.TrimSpace(key), model) {
				return append([]types.ReasoningLevel{}, levels...)
			}
		}
	}
	return append([]types.ReasoningLevel{}, catalog.ReasoningLevels...)
}

func (m *Model) modelDefaultReasoning(provider, model string) types.ReasoningLevel {
	catalog := m.providerOptionCatalog(provider)
	if catalog == nil {
		return ""
	}
	model = strings.TrimSpace(model)
	if model != "" && len(catalog.ModelDefaultReasoning) > 0 {
		for key, level := range catalog.ModelDefaultReasoning {
			if strings.EqualFold(strings.TrimSpace(key), model) {
				return level
			}
		}
	}
	return catalog.Defaults.Reasoning
}

func reasoningLevelAllowed(level types.ReasoningLevel, allowed []types.ReasoningLevel) bool {
	if level == "" || len(allowed) == 0 {
		return true
	}
	for _, entry := range allowed {
		if entry == level {
			return true
		}
	}
	return false
}

func (m *Model) normalizeComposeRuntimeOptionsForModel(provider string, options *types.SessionRuntimeOptions) {
	if options == nil {
		return
	}
	allowed := m.modelReasoningLevels(provider, options.Model)
	if len(allowed) == 0 {
		return
	}
	if reasoningLevelAllowed(options.Reasoning, allowed) {
		if options.Reasoning == "" {
			options.Reasoning = m.modelDefaultReasoning(provider, options.Model)
			if options.Reasoning == "" && len(allowed) > 0 {
				options.Reasoning = allowed[0]
			}
		}
		return
	}
	options.Reasoning = m.modelDefaultReasoning(provider, options.Model)
	if options.Reasoning == "" || !reasoningLevelAllowed(options.Reasoning, allowed) {
		options.Reasoning = allowed[0]
	}
}

func (m *Model) composeControlsRow() int {
	if m.chatInput == nil {
		return m.viewport.Height + 2
	}
	return m.viewport.Height + 2 + m.chatInput.Height()
}

func (m *Model) openComposeOptionPicker(target composeOptionKind) bool {
	if target == composeOptionNone || m.composeOptionPicker == nil {
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
	m.composeOptionPicker.SetOptions(options)
	m.composeOptionPicker.SelectID(selectedID)
	m.composeOptionTarget = target
	m.composeOptionSessionID = strings.TrimSpace(m.composeSessionID())
	m.composeOptionProvider = provider
	height := len(options)
	if height < 3 {
		height = 3
	}
	if height > 8 {
		height = 8
	}
	width := m.viewport.Width
	if width <= 0 {
		width = minViewportWidth
	}
	m.composeOptionPicker.SetSize(width, height)
	return true
}

func (m *Model) closeComposeOptionPicker() {
	m.composeOptionTarget = composeOptionNone
	m.composeOptionSessionID = ""
	m.composeOptionProvider = ""
}

func (m *Model) composeOptionPickerOpen() bool {
	return m.composeOptionTarget != composeOptionNone
}

func (m *Model) applyComposeOptionSelection(value string) tea.Cmd {
	target := m.composeOptionTarget
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

func (m *Model) composeOptionPopupView() (string, int) {
	if !m.composeOptionPickerOpen() || m.composeOptionPicker == nil {
		return "", 0
	}
	view := m.composeOptionPicker.View()
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
