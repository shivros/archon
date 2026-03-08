package app

import "testing"

func TestChatBubbleStylesUseSharedSymmetricPadding(t *testing.T) {
	styles := []struct {
		name  string
		style interface {
			GetPaddingTop() int
			GetPaddingBottom() int
			GetPaddingLeft() int
			GetPaddingRight() int
		}
	}{
		{name: "user", style: userBubbleStyle},
		{name: "agent", style: agentBubbleStyle},
		{name: "system", style: systemBubbleStyle},
		{name: "reasoning", style: reasoningBubbleStyle},
		{name: "approval", style: approvalBubbleStyle},
		{name: "approvalResolved", style: approvalResolvedBubbleStyle},
	}

	for _, tc := range styles {
		if got := tc.style.GetPaddingTop(); got != chatBubblePaddingVertical {
			t.Fatalf("%s padding top: expected %d, got %d", tc.name, chatBubblePaddingVertical, got)
		}
		if got := tc.style.GetPaddingBottom(); got != chatBubblePaddingVertical {
			t.Fatalf("%s padding bottom: expected %d, got %d", tc.name, chatBubblePaddingVertical, got)
		}
		if got := tc.style.GetPaddingLeft(); got != chatBubblePaddingHorizontal {
			t.Fatalf("%s padding left: expected %d, got %d", tc.name, chatBubblePaddingHorizontal, got)
		}
		if got := tc.style.GetPaddingRight(); got != chatBubblePaddingHorizontal {
			t.Fatalf("%s padding right: expected %d, got %d", tc.name, chatBubblePaddingHorizontal, got)
		}
	}
}

func TestThemePresetsIncludeRequiredBuiltins(t *testing.T) {
	required := map[string]struct{}{
		"default":         {},
		"nordic":          {},
		"gruvbox_dark":    {},
		"gruvbox_light":   {},
		"monokai":         {},
		"solarized_dark":  {},
		"solarized_light": {},
		"adwaita_dark":    {},
		"adwaita":         {},
	}
	for _, preset := range ThemePresets() {
		delete(required, preset.ID)
	}
	if len(required) > 0 {
		t.Fatalf("missing required theme presets: %#v", required)
	}
}

func TestApplyThemeFallsBackToDefaultForUnknownID(t *testing.T) {
	preset := ApplyTheme("does-not-exist")
	if preset.ID != defaultThemeID {
		t.Fatalf("expected unknown theme id to fallback to %q, got %q", defaultThemeID, preset.ID)
	}
	if got := CurrentThemeID(); got != defaultThemeID {
		t.Fatalf("expected current theme id %q, got %q", defaultThemeID, got)
	}
}

func TestApplyThemeUpdatesProviderBadgeDefaults(t *testing.T) {
	_ = ApplyTheme("default")
	defaultCodex := resolveProviderBadge("codex", nil).Color
	_ = ApplyTheme("monokai")
	monokaiCodex := resolveProviderBadge("codex", nil).Color
	if monokaiCodex == "" {
		t.Fatalf("expected codex color to be set after monokai apply")
	}
	if monokaiCodex == defaultCodex {
		t.Fatalf("expected codex badge color to change across themes, got %q", monokaiCodex)
	}
}

func TestThemePresetDerivationIncludesInfoToastColor(t *testing.T) {
	for _, preset := range ThemePresets() {
		_ = ApplyTheme(preset.ID)
		if toastInfoStyle.GetBackground() == nil {
			t.Fatalf("expected info toast background for theme %q", preset.ID)
		}
	}
}
