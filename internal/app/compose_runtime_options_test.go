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
