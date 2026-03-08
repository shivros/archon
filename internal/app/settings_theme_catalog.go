package app

import "strings"

func defaultSettingsThemeItemsFromCatalog() []SettingsThemeItem {
	presets := ThemePresets()
	out := make([]SettingsThemeItem, 0, len(presets))
	for _, preset := range presets {
		out = append(out, SettingsThemeItem{
			ID:    normalizeSettingsThemeID(preset.ID),
			Title: strings.TrimSpace(preset.Label),
		})
	}
	return out
}
