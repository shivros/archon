package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"control/internal/config"
)

type ThemePreferenceStore interface {
	SaveTheme(themeID string) error
}

type fileThemePreferenceStore struct{}

func (fileThemePreferenceStore) SaveTheme(themeID string) error {
	return config.UpdateUITheme(themeID)
}

func WithThemePreferenceStore(store ThemePreferenceStore) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		m.themePreferenceStore = store
	}
}

func (m *Model) themePreferenceStoreOrDefault() ThemePreferenceStore {
	if m == nil || m.themePreferenceStore == nil {
		return fileThemePreferenceStore{}
	}
	return m.themePreferenceStore
}

func saveThemePreferenceCmd(store ThemePreferenceStore, themeID string) tea.Cmd {
	resolved := normalizeThemeID(themeID)
	return func() tea.Msg {
		if resolved == "" {
			return themePreferenceSavedMsg{}
		}
		if store == nil {
			store = fileThemePreferenceStore{}
		}
		err := store.SaveTheme(resolved)
		return themePreferenceSavedMsg{themeID: resolved, err: err}
	}
}

func (m *Model) applyThemeSelection(themeID string) tea.Cmd {
	preset := ApplyTheme(themeID)
	previous := m.themeID
	m.themeID = preset.ID
	if m.settingsMenu != nil {
		m.settingsMenu.SetActiveThemeID(m.themeID)
		m.settingsMenu.SetSelectedThemeID(m.themeID)
	}
	if m.width > 0 && m.height > 0 && previous != m.themeID {
		m.renderViewport()
		m.renderNotesPanel()
	}
	label := strings.TrimSpace(preset.Label)
	if label == "" {
		label = strings.ToUpper(preset.ID)
	}
	m.setStatusInfo("theme set: " + label)
	return saveThemePreferenceCmd(m.themePreferenceStoreOrDefault(), preset.ID)
}
