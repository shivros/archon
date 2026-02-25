package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"control/internal/types"
)

func (m *Model) composeProvider() string {
	if m != nil && m.mode == uiModeGuidedWorkflow && m.guidedWorkflow != nil && m.guidedWorkflow.Stage() == guidedWorkflowStageSetup {
		return strings.TrimSpace(m.guidedWorkflow.Provider())
	}
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
	if m != nil && m.mode == uiModeGuidedWorkflow && m.guidedWorkflow != nil && m.guidedWorkflow.Stage() == guidedWorkflowStageSetup {
		var out *types.SessionRuntimeOptions
		if m.newSession != nil && strings.EqualFold(strings.TrimSpace(m.newSession.provider), provider) {
			out = types.CloneRuntimeOptions(m.newSession.runtimeOptions)
		}
		if out == nil {
			out = m.guidedWorkflow.RuntimeOptions()
		}
		if out == nil {
			out = m.composeDefaultsForProvider(provider)
		}
		if out == nil {
			out = &types.SessionRuntimeOptions{}
		}
		return m.resolveComposeRuntimeOptions(provider, out)
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
	return m.resolveComposeRuntimeOptions(provider, out)
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
	if m == nil || m.chatAddonController == nil {
		return ""
	}
	return m.chatAddonController.composeControlsLine(m)
}

func (m *Model) composeOptionResolver() runtimeOptionResolver {
	return newRuntimeOptionResolver(m, m)
}

func (m *Model) resolveComposeRuntimeOptions(provider string, options *types.SessionRuntimeOptions) *types.SessionRuntimeOptions {
	if m == nil {
		return types.CloneRuntimeOptions(options)
	}
	return m.composeOptionResolver().resolve(provider, options)
}

func (m *Model) modelReasoningLevels(provider, model string) []types.ReasoningLevel {
	catalog := m.providerOptionCatalog(provider)
	return runtimeReasoningLevelsForModel(catalog, model)
}

func (m *Model) normalizeComposeRuntimeOptionsForModel(provider string, options *types.SessionRuntimeOptions) {
	if options == nil {
		return
	}
	resolved := m.resolveComposeRuntimeOptions(provider, options)
	options.Reasoning = resolved.Reasoning
}

func (m *Model) normalizeComposeRuntimeOptionsForProvider(provider string, options *types.SessionRuntimeOptions) {
	if options == nil {
		return
	}
	resolved := m.resolveComposeRuntimeOptions(provider, options)
	*options = *resolved
}

func (m *Model) composeControlsRow() int {
	start := m.viewport.Height() + 2
	if m == nil {
		return start
	}
	if m.mode != uiModeCompose && !(m.mode == uiModeGuidedWorkflow && m.guidedWorkflow != nil && m.guidedWorkflow.Stage() == guidedWorkflowStageSetup) {
		return start
	}
	layout, ok := m.activeInputPanelLayout()
	if !ok {
		if m.mode == uiModeGuidedWorkflow {
			layout, ok = m.guidedWorkflowSetupInputPanelLayout()
		}
	}
	if !ok {
		return start
	}
	if row, ok := layout.FooterStartRow(); ok {
		return start + row
	}
	return start + layout.InputLineCount()
}

func (m *Model) guidedWorkflowSetupInputPanelLayout() (InputPanelLayout, bool) {
	panel, ok := m.guidedWorkflowSetupInputPanel()
	if !ok {
		return InputPanelLayout{}, false
	}
	return BuildInputPanelLayout(panel), true
}

func (m *Model) openComposeOptionPicker(target composeOptionKind) bool {
	if m == nil || m.chatAddonController == nil {
		return false
	}
	return m.chatAddonController.openComposeOptionPicker(m, target)
}

func (m *Model) requestComposeOptionPicker(target composeOptionKind) tea.Cmd {
	if m == nil || target == composeOptionNone {
		return nil
	}
	provider := strings.ToLower(strings.TrimSpace(m.composeProvider()))
	if provider == "" {
		return nil
	}
	needsRefresh := m.shouldRefreshComposeOptions(provider, target)
	if needsRefresh {
		m.pendingComposeOptionTarget = target
		m.pendingComposeOptionFor = provider
		m.setStatusMessage("loading " + composeOptionLabel(target) + " options")
		ctx := m.replaceRequestScope(requestScopeProviderOption)
		return fetchProviderOptionsCmdWithContext(m.sessionAPI, provider, ctx)
	}
	if m.openComposeOptionPicker(target) {
		m.setStatusMessage("select " + composeOptionLabel(target))
		return nil
	}
	// If options are missing or stale, fetch and reopen automatically.
	m.pendingComposeOptionTarget = target
	m.pendingComposeOptionFor = provider
	m.setStatusMessage("loading " + composeOptionLabel(target) + " options")
	ctx := m.replaceRequestScope(requestScopeProviderOption)
	return fetchProviderOptionsCmdWithContext(m.sessionAPI, provider, ctx)
}

func (m *Model) shouldRefreshComposeOptions(provider string, target composeOptionKind) bool {
	if m == nil || target == composeOptionNone {
		return false
	}
	if m.providerOptionCatalog(provider) == nil {
		return true
	}
	if target != composeOptionModel {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "opencode", "kilocode":
		return true
	default:
		return false
	}
}

func composeOptionLabel(target composeOptionKind) string {
	switch target {
	case composeOptionModel:
		return "model"
	case composeOptionReasoning:
		return "reasoning"
	case composeOptionAccess:
		return "access"
	default:
		return "session"
	}
}

func (m *Model) closeComposeOptionPicker() {
	if m == nil || m.chatAddonController == nil {
		return
	}
	m.chatAddonController.closeComposeOptionPicker()
}

func (m *Model) composeOptionPickerOpen() bool {
	if m == nil || m.chatAddonController == nil {
		return false
	}
	return m.chatAddonController.composeOptionPickerOpen()
}

func (m *Model) applyComposeOptionSelection(value string) tea.Cmd {
	if m == nil || m.chatAddonController == nil {
		return nil
	}
	return m.chatAddonController.applyComposeOptionSelection(m, value)
}

func (m *Model) composeOptionPopupView() (string, int) {
	if m == nil || m.chatAddonController == nil {
		return "", 0
	}
	return m.chatAddonController.composeOptionPopupView(m)
}

func (m *Model) composeOptionPickerSelectedID() string {
	if m == nil || m.chatAddonController == nil {
		return ""
	}
	return m.chatAddonController.composeOptionPickerSelectedID()
}

func (m *Model) composeOptionPickerQuery() string {
	if m == nil || m.chatAddonController == nil {
		return ""
	}
	return m.chatAddonController.composeOptionPickerQuery()
}

func (m *Model) composeOptionPickerAppendQuery(text string) bool {
	if m == nil || m.chatAddonController == nil {
		return false
	}
	return m.chatAddonController.composeOptionPickerAppendQuery(text)
}

func (m *Model) composeOptionPickerBackspaceQuery() bool {
	if m == nil || m.chatAddonController == nil {
		return false
	}
	return m.chatAddonController.composeOptionPickerBackspaceQuery()
}

func (m *Model) composeOptionPickerClearQuery() bool {
	if m == nil || m.chatAddonController == nil {
		return false
	}
	return m.chatAddonController.composeOptionPickerClearQuery()
}

func (m *Model) moveComposeOptionPicker(delta int) {
	if m == nil || m.chatAddonController == nil {
		return
	}
	m.chatAddonController.moveComposeOptionPicker(delta)
}

func (m *Model) composeOptionPickerHandleClick(row int) bool {
	if m == nil || m.chatAddonController == nil {
		return false
	}
	return m.chatAddonController.composeOptionPickerHandleClick(row)
}

func (m *Model) composeControlSpans() []composeControlSpan {
	if m == nil || m.chatAddonController == nil {
		return nil
	}
	return m.chatAddonController.composeControlSpans()
}

func (m *Model) clearPendingComposeOptionRequest() {
	if m == nil {
		return
	}
	m.pendingComposeOptionTarget = composeOptionNone
	m.pendingComposeOptionFor = ""
	m.cancelRequestScope(requestScopeProviderOption)
}
