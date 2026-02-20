package app

import (
	"strings"
	"testing"

	xansi "github.com/charmbracelet/x/ansi"

	"control/internal/types"
)

func TestComposeControlsLineShowsRuntimeOptions(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeCompose
	m.newSession = &newSessionTarget{provider: "codex"}

	line := m.composeControlsLine()
	if !strings.Contains(line, "Model:") {
		t.Fatalf("expected model control, got %q", line)
	}
	if !strings.Contains(line, "Reasoning:") {
		t.Fatalf("expected reasoning control, got %q", line)
	}
	if !strings.Contains(line, "Access:") {
		t.Fatalf("expected access control, got %q", line)
	}
}

func TestComposeControlsLineHidesReasoningWhenProviderDoesNotSupportIt(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeCompose
	m.newSession = &newSessionTarget{provider: "claude"}

	line := m.composeControlsLine()
	if !strings.Contains(line, "Model:") {
		t.Fatalf("expected model control, got %q", line)
	}
	if strings.Contains(line, "Reasoning:") {
		t.Fatalf("expected reasoning control to be hidden for claude, got %q", line)
	}
	if !strings.Contains(line, "Access:") {
		t.Fatalf("expected access control, got %q", line)
	}
}

func TestOpenComposeOptionPickerReasoningDisabledWhenProviderHasNoReasoningLevels(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeCompose
	m.newSession = &newSessionTarget{provider: "claude"}
	if m.openComposeOptionPicker(composeOptionReasoning) {
		t.Fatalf("expected reasoning picker to remain closed for claude")
	}
}

func TestApplyComposeOptionSelectionUpdatesNewSessionDefaults(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeCompose
	m.newSession = &newSessionTarget{provider: "codex"}
	if !m.openComposeOptionPicker(composeOptionAccess) {
		t.Fatalf("expected access option picker to open")
	}

	_ = m.applyComposeOptionSelection(string(types.AccessFull))
	if m.newSession.runtimeOptions == nil {
		t.Fatalf("expected runtime options on new session")
	}
	if m.newSession.runtimeOptions.Access != types.AccessFull {
		t.Fatalf("expected full access, got %q", m.newSession.runtimeOptions.Access)
	}
	if m.appState.ComposeDefaultsByProvider == nil {
		t.Fatalf("expected compose defaults to persist")
	}
	if got := m.appState.ComposeDefaultsByProvider["codex"]; got == nil || got.Access != types.AccessFull {
		t.Fatalf("expected codex defaults to store full access, got %#v", got)
	}
}

func TestApplyComposeOptionSelectionModelAdjustsReasoningByModel(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeCompose
	m.newSession = &newSessionTarget{
		provider: "codex",
		runtimeOptions: &types.SessionRuntimeOptions{
			Model:     "gpt-5.2-codex",
			Reasoning: types.ReasoningExtraHigh,
		},
	}
	m.providerOptions = map[string]*types.ProviderOptionCatalog{
		"codex": {
			Provider: "codex",
			Models:   []string{"gpt-5.2-codex", "gpt-5.3-codex"},
			ModelReasoningLevels: map[string][]types.ReasoningLevel{
				"gpt-5.2-codex": {types.ReasoningLow},
				"gpt-5.3-codex": {types.ReasoningMedium, types.ReasoningHigh},
			},
			ModelDefaultReasoning: map[string]types.ReasoningLevel{
				"gpt-5.2-codex": types.ReasoningLow,
				"gpt-5.3-codex": types.ReasoningMedium,
			},
			ReasoningLevels: []types.ReasoningLevel{types.ReasoningLow, types.ReasoningMedium, types.ReasoningHigh},
			AccessLevels:    []types.AccessLevel{types.AccessOnRequest},
			Defaults:        types.SessionRuntimeOptions{Model: "gpt-5.2-codex", Reasoning: types.ReasoningLow, Access: types.AccessOnRequest},
		},
	}
	if !m.openComposeOptionPicker(composeOptionModel) {
		t.Fatalf("expected model option picker to open")
	}

	_ = m.applyComposeOptionSelection("gpt-5.2-codex")
	if m.newSession.runtimeOptions == nil {
		t.Fatalf("expected runtime options to exist")
	}
	if m.newSession.runtimeOptions.Reasoning != types.ReasoningLow {
		t.Fatalf("expected reasoning to auto-adjust to model default, got %q", m.newSession.runtimeOptions.Reasoning)
	}
}

func TestComposeOptionPopupViewOffsetsForSidebar(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.mode = uiModeCompose
	m.appState.SidebarCollapsed = false
	m.newSession = &newSessionTarget{provider: "codex"}

	if !m.openComposeOptionPicker(composeOptionModel) {
		t.Fatalf("expected model option picker to open")
	}
	popup, _ := m.composeOptionPopupView()
	if popup == "" {
		t.Fatalf("expected popup content")
	}
	layout := m.resolveMouseLayout()
	if layout.rightStart <= 0 {
		t.Fatalf("expected right pane offset when sidebar is open, got %d", layout.rightStart)
	}
	line := xansi.Strip(strings.Split(popup, "\n")[0])
	prefix := strings.Repeat(" ", layout.rightStart)
	if !strings.HasPrefix(line, prefix) {
		t.Fatalf("expected popup line to start with %d spaces for sidebar offset, got %q", layout.rightStart, line)
	}
}

func TestRequestComposeOptionPickerFetchesAndAutoOpensForOpenCodeModel(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.mode = uiModeCompose
	m.newSession = &newSessionTarget{provider: "opencode"}

	cmd := m.requestComposeOptionPicker(composeOptionModel)
	if cmd == nil {
		t.Fatalf("expected provider options fetch command")
	}
	if m.composeOptionPickerOpen() {
		t.Fatalf("expected picker to stay closed until options load")
	}
	if m.pendingComposeOptionTarget != composeOptionModel || m.pendingComposeOptionFor != "opencode" {
		t.Fatalf("expected pending compose option request to be tracked")
	}

	nextModel, follow := m.Update(providerOptionsMsg{
		provider: "opencode",
		options: &types.ProviderOptionCatalog{
			Provider: "opencode",
			Models:   []string{"anthropic/claude-sonnet-4-20250514", "openai/gpt-5"},
			Defaults: types.SessionRuntimeOptions{Model: "anthropic/claude-sonnet-4-20250514"},
		},
	})
	next, ok := nextModel.(*Model)
	if !ok || next == nil {
		t.Fatalf("expected model update result, got %T", nextModel)
	}
	if follow != nil {
		t.Fatalf("expected no follow-up command, got %T", follow)
	}
	if !next.composeOptionPickerOpen() {
		t.Fatalf("expected model option picker to auto-open once options load")
	}
	if next.pendingComposeOptionTarget != composeOptionNone || next.pendingComposeOptionFor != "" {
		t.Fatalf("expected pending compose option request to clear")
	}
}

func TestComposeOptionPickerTypeAheadFiltersModelOptions(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeCompose
	m.newSession = &newSessionTarget{provider: "codex"}
	if !m.openComposeOptionPicker(composeOptionModel) {
		t.Fatalf("expected model option picker to open")
	}
	if !m.composeOptionPickerAppendQuery("53c") {
		t.Fatalf("expected type-ahead query to update picker")
	}
	selected := m.composeOptionPickerSelectedID()
	if selected != "gpt-5.3-codex" {
		t.Fatalf("expected filtered model selection to be gpt-5.3-codex, got %q", selected)
	}
	_ = m.applyComposeOptionSelection(selected)
	if m.newSession.runtimeOptions == nil || m.newSession.runtimeOptions.Model != "gpt-5.3-codex" {
		t.Fatalf("expected selected model to be applied, got %#v", m.newSession.runtimeOptions)
	}
}

func TestComposeControlsRowUsesFooterStartRowWhenFooterVisible(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeCompose
	m.newSession = &newSessionTarget{provider: "codex"}
	m.resize(120, 40)

	layout, ok := m.activeInputPanelLayout()
	if !ok {
		t.Fatalf("expected compose input layout")
	}
	footerStart, ok := layout.FooterStartRow()
	if !ok {
		t.Fatalf("expected compose footer row")
	}
	want := m.viewport.Height() + 2 + footerStart
	if got := m.composeControlsRow(); got != want {
		t.Fatalf("expected compose controls row %d, got %d", want, got)
	}
}

func TestComposeControlsRowFallsBackToInputBottomWhenFooterHidden(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeCompose
	m.resize(120, 40)
	if line := m.composeControlsLine(); line != "" {
		t.Fatalf("expected compose controls line to be hidden without provider context, got %q", line)
	}

	layout, ok := m.activeInputPanelLayout()
	if !ok {
		t.Fatalf("expected compose input layout")
	}
	if _, ok := layout.FooterStartRow(); ok {
		t.Fatalf("did not expect compose footer row when controls are hidden")
	}
	want := m.viewport.Height() + 2 + layout.InputLineCount()
	if got := m.composeControlsRow(); got != want {
		t.Fatalf("expected compose controls row fallback %d, got %d", want, got)
	}
}

func TestComposeControlsRowReturnsStartOutsideComposeMode(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.mode = uiModeNormal

	want := m.viewport.Height() + 2
	if got := m.composeControlsRow(); got != want {
		t.Fatalf("expected compose controls row start %d outside compose mode, got %d", want, got)
	}
}

func TestComposeControlsRowReturnsStartWhenComposeInputMissing(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeCompose
	m.chatInput = nil
	m.resize(120, 40)

	want := m.viewport.Height() + 2
	if got := m.composeControlsRow(); got != want {
		t.Fatalf("expected compose controls row start %d when input missing, got %d", want, got)
	}
}
