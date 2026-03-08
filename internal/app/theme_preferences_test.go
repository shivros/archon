package app

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"control/internal/config"
)

type fakeThemePreferenceStore struct {
	saved []string
	err   error
}

func (f *fakeThemePreferenceStore) SaveTheme(themeID string) error {
	f.saved = append(f.saved, themeID)
	return f.err
}

func TestReduceSettingsMenuThemeApplyPersistsSelection(t *testing.T) {
	store := &fakeThemePreferenceStore{}
	m := NewModel(nil, WithThemePreferenceStore(store))
	m.settingsMenu.Open()

	_, _ = m.reduceSettingsMenu(tea.KeyPressMsg{Code: tea.KeyDown})  // THEME
	_, _ = m.reduceSettingsMenu(tea.KeyPressMsg{Code: tea.KeyEnter}) // open THEME screen
	_, _ = m.reduceSettingsMenu(tea.KeyPressMsg{Code: tea.KeyDown})  // Nordic
	handled, cmd := m.reduceSettingsMenu(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled {
		t.Fatalf("expected theme apply key to be handled")
	}
	if cmd == nil {
		t.Fatalf("expected theme apply to return persistence cmd")
	}
	if got := m.themeID; got != "nordic" {
		t.Fatalf("expected applied theme nordic, got %q", got)
	}
	msg := cmd()
	saved, ok := msg.(themePreferenceSavedMsg)
	if !ok {
		t.Fatalf("expected themePreferenceSavedMsg, got %T", msg)
	}
	if saved.err != nil {
		t.Fatalf("expected save success, got err=%v", saved.err)
	}
	if len(store.saved) != 1 || store.saved[0] != "nordic" {
		t.Fatalf("expected save call for nordic, got %#v", store.saved)
	}
}

func TestThemePreferenceSavedMsgErrorSetsStatus(t *testing.T) {
	m := NewModel(nil)
	m.themeID = "default"
	handled, cmd := m.reduceStateMessages(themePreferenceSavedMsg{themeID: "default", err: errors.New("boom")})
	if !handled {
		t.Fatalf("expected theme preference message to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no follow-up cmd")
	}
	if !strings.Contains(m.status, "theme save error: boom") {
		t.Fatalf("expected theme save error status, got %q", m.status)
	}
}

func TestFileThemePreferenceStoreSaveThemePersistsToUIConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	store := fileThemePreferenceStore{}
	if err := store.SaveTheme("Solarized Light"); err != nil {
		t.Fatalf("SaveTheme: %v", err)
	}

	cfg, err := config.LoadUIConfig()
	if err != nil {
		t.Fatalf("LoadUIConfig: %v", err)
	}
	if got := cfg.ThemeName(); got != "solarized_light" {
		t.Fatalf("expected persisted theme solarized_light, got %q", got)
	}
}

func TestSaveThemePreferenceCmdFallsBackToFileStoreWhenNil(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cmd := saveThemePreferenceCmd(nil, " Gruvbox Light ")
	if cmd == nil {
		t.Fatalf("expected save cmd")
	}
	msg := cmd()
	saved, ok := msg.(themePreferenceSavedMsg)
	if !ok {
		t.Fatalf("expected themePreferenceSavedMsg, got %T", msg)
	}
	if saved.err != nil {
		t.Fatalf("expected save success, got %v", saved.err)
	}
	if saved.themeID != "gruvbox_light" {
		t.Fatalf("expected normalized saved theme id, got %q", saved.themeID)
	}

	cfg, err := config.LoadUIConfig()
	if err != nil {
		t.Fatalf("LoadUIConfig: %v", err)
	}
	if got := cfg.ThemeName(); got != "gruvbox_light" {
		t.Fatalf("expected persisted theme gruvbox_light, got %q", got)
	}
}

func TestApplyThemeSelectionHandlesNilSettingsMenu(t *testing.T) {
	store := &fakeThemePreferenceStore{}
	m := NewModel(nil, WithThemePreferenceStore(store))
	m.settingsMenu = nil
	m.width = 80
	m.height = 24
	m.themeID = "default"

	cmd := m.applyThemeSelection("Monokai")
	if cmd == nil {
		t.Fatalf("expected save cmd")
	}
	if got := m.themeID; got != "monokai" {
		t.Fatalf("expected theme id monokai, got %q", got)
	}
	if !strings.Contains(m.status, "theme set: Monokai") {
		t.Fatalf("expected status update for monokai, got %q", m.status)
	}

	msg := cmd()
	saved, ok := msg.(themePreferenceSavedMsg)
	if !ok {
		t.Fatalf("expected themePreferenceSavedMsg, got %T", msg)
	}
	if saved.err != nil {
		t.Fatalf("expected save success, got %v", saved.err)
	}
	if len(store.saved) != 1 || store.saved[0] != "monokai" {
		t.Fatalf("expected save call for monokai, got %#v", store.saved)
	}
}

func TestThemeApplyRoundTripPersistsAndRehydratesFromUIConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	m := NewModel(nil)
	m.settingsMenu.Open()
	_, _ = m.reduceSettingsMenu(tea.KeyPressMsg{Code: tea.KeyDown})  // THEME
	_, _ = m.reduceSettingsMenu(tea.KeyPressMsg{Code: tea.KeyEnter}) // open THEME screen
	_, _ = m.reduceSettingsMenu(tea.KeyPressMsg{Code: tea.KeyDown})  // Nordic
	handled, cmd := m.reduceSettingsMenu(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled || cmd == nil {
		t.Fatalf("expected handled apply with persistence cmd, got handled=%t cmd=%v", handled, cmd)
	}
	msg := cmd()
	saved, ok := msg.(themePreferenceSavedMsg)
	if !ok {
		t.Fatalf("expected themePreferenceSavedMsg, got %T", msg)
	}
	if saved.err != nil {
		t.Fatalf("expected save success, got %v", saved.err)
	}

	cfg, err := config.LoadUIConfig()
	if err != nil {
		t.Fatalf("LoadUIConfig: %v", err)
	}
	if got := cfg.ThemeName(); got != "nordic" {
		t.Fatalf("expected persisted theme nordic, got %q", got)
	}

	m2 := NewModel(nil)
	m2.applyUIConfig(cfg)
	if got := m2.themeID; got != "nordic" {
		t.Fatalf("expected model theme from config to be nordic, got %q", got)
	}
}
