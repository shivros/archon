package app

import (
	"strconv"
	"strings"
)

var themePresets = buildThemePresets()

func init() {
	applyTheme(resolveThemePreset(defaultThemeID))
}

func ThemePresets() []ThemePreset {
	out := make([]ThemePreset, 0, len(themePresets))
	for _, preset := range themePresets {
		out = append(out, ThemePreset{ID: preset.ID, Label: preset.Label, palette: preset.palette})
	}
	return out
}

func CurrentThemeID() string {
	return activeThemeID
}

func ApplyTheme(themeID string) ThemePreset {
	preset := resolveThemePreset(themeID)
	applyTheme(preset)
	return preset
}

func normalizeThemeID(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	value = strings.ReplaceAll(value, "-", "_")
	value = strings.ReplaceAll(value, " ", "_")
	if value == "" {
		return defaultThemeID
	}
	return value
}

func resolveThemePreset(themeID string) ThemePreset {
	normalized := normalizeThemeID(themeID)
	for _, preset := range themePresets {
		if preset.ID == normalized {
			return preset
		}
	}
	return themePresets[0]
}
func buildThemePresets() []ThemePreset {
	defaultPalette := defaultThemePalette()
	return []ThemePreset{
		{ID: "default", Label: "Default", palette: defaultPalette},
		{ID: "nordic", Label: "Nordic", palette: paletteFromScheme(themeScheme{
			FgPrimary:       "#D8DEE9",
			FgMuted:         "#A3B2C3",
			FgDim:           "#8CA2BA",
			FgStrong:        "#ECEFF4",
			BgBase:          "#2E3440",
			BgElevated:      "#3B4252",
			BgSubtle:        "#434C5E",
			BgSelection:     "#4C566A",
			BgSelectionAlt:  "#5E81AC",
			Accent:          "#88C0D0",
			AccentAlt:       "#81A1C1",
			Success:         "#A3BE8C",
			Warning:         "#EBCB8B",
			Danger:          "#BF616A",
			Border:          "#4C566A",
			BorderAccent:    "#88C0D0",
			SortMuted:       "#81A1C1",
			SortActiveBg:    "#5E81AC",
			Workspace:       "#8FBCBB",
			WorkspaceActive: "#88C0D0",
			Worktree:        "#81A1C1",
			WorktreeActive:  "#5E81AC",
			SessionUnread:   "#A3BE8C",
		})},
		{ID: "gruvbox_dark", Label: "Gruvbox Dark", palette: paletteFromScheme(themeScheme{
			FgPrimary:       "#EBDBB2",
			FgMuted:         "#A89984",
			FgDim:           "#928374",
			FgStrong:        "#FBF1C7",
			BgBase:          "#282828",
			BgElevated:      "#3C3836",
			BgSubtle:        "#32302F",
			BgSelection:     "#504945",
			BgSelectionAlt:  "#665C54",
			Accent:          "#83A598",
			AccentAlt:       "#8EC07C",
			Success:         "#B8BB26",
			Warning:         "#FABD2F",
			Danger:          "#FB4934",
			Border:          "#665C54",
			BorderAccent:    "#83A598",
			SortMuted:       "#928374",
			SortActiveBg:    "#665C54",
			Workspace:       "#D3869B",
			WorkspaceActive: "#FE8019",
			Worktree:        "#8EC07C",
			WorktreeActive:  "#B8BB26",
			SessionUnread:   "#B8BB26",
		})},
		{ID: "gruvbox_light", Label: "Gruvbox Light", palette: paletteFromScheme(themeScheme{
			FgPrimary:       "#3C3836",
			FgMuted:         "#7C6F64",
			FgDim:           "#928374",
			FgStrong:        "#282828",
			BgBase:          "#FBF1C7",
			BgElevated:      "#EBDBB2",
			BgSubtle:        "#F2E5BC",
			BgSelection:     "#D5C4A1",
			BgSelectionAlt:  "#BDAE93",
			Accent:          "#076678",
			AccentAlt:       "#427B58",
			Success:         "#79740E",
			Warning:         "#B57614",
			Danger:          "#9D0006",
			Border:          "#BDAE93",
			BorderAccent:    "#076678",
			SortMuted:       "#7C6F64",
			SortActiveBg:    "#D5C4A1",
			Workspace:       "#8F3F71",
			WorkspaceActive: "#AF3A03",
			Worktree:        "#427B58",
			WorktreeActive:  "#79740E",
			SessionUnread:   "#79740E",
		})},
		{ID: "monokai", Label: "Monokai", palette: paletteFromScheme(themeScheme{
			FgPrimary:       "#F8F8F2",
			FgMuted:         "#BFBFC0",
			FgDim:           "#95979C",
			FgStrong:        "#FFFFFF",
			BgBase:          "#272822",
			BgElevated:      "#3A3B35",
			BgSubtle:        "#2F3129",
			BgSelection:     "#49483E",
			BgSelectionAlt:  "#5A5B52",
			Accent:          "#66D9EF",
			AccentAlt:       "#A6E22E",
			Success:         "#A6E22E",
			Warning:         "#E6DB74",
			Danger:          "#F92672",
			Border:          "#49483E",
			BorderAccent:    "#66D9EF",
			SortMuted:       "#A59F85",
			SortActiveBg:    "#5A5B52",
			Workspace:       "#AE81FF",
			WorkspaceActive: "#FD971F",
			Worktree:        "#A6E22E",
			WorktreeActive:  "#66D9EF",
			SessionUnread:   "#A6E22E",
		})},
		{ID: "solarized_dark", Label: "Solarized Dark", palette: paletteFromScheme(themeScheme{
			FgPrimary:       "#93A1A1",
			FgMuted:         "#839496",
			FgDim:           "#657B83",
			FgStrong:        "#EEE8D5",
			BgBase:          "#002B36",
			BgElevated:      "#073642",
			BgSubtle:        "#003541",
			BgSelection:     "#264653",
			BgSelectionAlt:  "#2E5D6B",
			Accent:          "#268BD2",
			AccentAlt:       "#2AA198",
			Success:         "#859900",
			Warning:         "#B58900",
			Danger:          "#DC322F",
			Border:          "#586E75",
			BorderAccent:    "#268BD2",
			SortMuted:       "#657B83",
			SortActiveBg:    "#2E5D6B",
			Workspace:       "#6C71C4",
			WorkspaceActive: "#268BD2",
			Worktree:        "#2AA198",
			WorktreeActive:  "#859900",
			SessionUnread:   "#859900",
		})},
		{ID: "solarized_light", Label: "Solarized Light", palette: paletteFromScheme(themeScheme{
			FgPrimary:       "#586E75",
			FgMuted:         "#657B83",
			FgDim:           "#839496",
			FgStrong:        "#073642",
			BgBase:          "#FDF6E3",
			BgElevated:      "#EEE8D5",
			BgSubtle:        "#F5EFD9",
			BgSelection:     "#E4DDC8",
			BgSelectionAlt:  "#D7D0BC",
			Accent:          "#268BD2",
			AccentAlt:       "#2AA198",
			Success:         "#859900",
			Warning:         "#B58900",
			Danger:          "#DC322F",
			Border:          "#93A1A1",
			BorderAccent:    "#268BD2",
			SortMuted:       "#839496",
			SortActiveBg:    "#D7D0BC",
			Workspace:       "#6C71C4",
			WorkspaceActive: "#268BD2",
			Worktree:        "#2AA198",
			WorktreeActive:  "#859900",
			SessionUnread:   "#859900",
		})},
		{ID: "adwaita_dark", Label: "Adwaita Dark", palette: paletteFromScheme(themeScheme{
			FgPrimary:       "#EEEEEC",
			FgMuted:         "#C0BFBC",
			FgDim:           "#9A9996",
			FgStrong:        "#FFFFFF",
			BgBase:          "#1E1E1E",
			BgElevated:      "#2A2A2A",
			BgSubtle:        "#252525",
			BgSelection:     "#3A3A3A",
			BgSelectionAlt:  "#4A4A4A",
			Accent:          "#3584E4",
			AccentAlt:       "#33D17A",
			Success:         "#33D17A",
			Warning:         "#F6D32D",
			Danger:          "#E01B24",
			Border:          "#4A4A4A",
			BorderAccent:    "#3584E4",
			SortMuted:       "#9A9996",
			SortActiveBg:    "#4A4A4A",
			Workspace:       "#62A0EA",
			WorkspaceActive: "#3584E4",
			Worktree:        "#33D17A",
			WorktreeActive:  "#57E389",
			SessionUnread:   "#33D17A",
		})},
		{ID: "adwaita", Label: "Adwaita", palette: paletteFromScheme(themeScheme{
			FgPrimary:       "#3D3846",
			FgMuted:         "#5E5C64",
			FgDim:           "#77767B",
			FgStrong:        "#241F31",
			BgBase:          "#FAFAFA",
			BgElevated:      "#F2F2F2",
			BgSubtle:        "#F7F7F7",
			BgSelection:     "#E3E3E3",
			BgSelectionAlt:  "#D7D7D7",
			Accent:          "#1C71D8",
			AccentAlt:       "#26A269",
			Success:         "#2EC27E",
			Warning:         "#E5A50A",
			Danger:          "#C01C28",
			Border:          "#D7D7D7",
			BorderAccent:    "#1C71D8",
			SortMuted:       "#77767B",
			SortActiveBg:    "#D7D7D7",
			Workspace:       "#1A5FB4",
			WorkspaceActive: "#1C71D8",
			Worktree:        "#26A269",
			WorktreeActive:  "#2EC27E",
			SessionUnread:   "#26A269",
		})},
	}
}

type themeScheme struct {
	FgPrimary       string
	FgMuted         string
	FgDim           string
	FgStrong        string
	BgBase          string
	BgElevated      string
	BgSubtle        string
	BgSelection     string
	BgSelectionAlt  string
	Accent          string
	AccentAlt       string
	Success         string
	Warning         string
	Danger          string
	Border          string
	BorderAccent    string
	SortMuted       string
	SortActiveBg    string
	Workspace       string
	WorkspaceActive string
	Worktree        string
	WorktreeActive  string
	SessionUnread   string
}

func paletteFromScheme(s themeScheme) themePalette {
	p := defaultThemePalette()
	p.MainPaneBg = s.BgBase
	p.SidebarPaneBg = s.BgElevated
	p.HeaderFg = s.Accent
	p.StatusFg = s.FgMuted
	p.StatusHistorySelectedFg = s.FgStrong
	p.StatusHistorySelectedBg = s.BgSelectionAlt
	p.ActivityFg = s.Worktree
	p.WorkspaceFg = s.Workspace
	p.WorkspaceActiveFg = s.WorkspaceActive
	p.WorktreeFg = s.Worktree
	p.WorktreeActiveFg = s.WorktreeActive
	p.SessionFg = s.FgPrimary
	p.SessionUnreadFg = s.SessionUnread
	p.SelectedFg = s.FgStrong
	p.SelectedBg = s.BgSelection
	p.MultiSelectFg = s.FgStrong
	p.MultiSelectBg = s.BgSelectionAlt
	p.HighlightRowFg = s.FgStrong
	p.HighlightRowBg = s.BgSelectionAlt
	p.DividerFg = s.Border
	p.ScrollbarTrackFg = s.Border
	p.ScrollbarThumbFg = s.FgMuted
	p.MenuBarFg = s.FgPrimary
	p.MenuBarBg = s.BgSelection
	p.MenuBarActiveFg = s.FgStrong
	p.MenuBarActiveBg = s.BgSelectionAlt
	p.MenuDropFg = s.FgPrimary
	p.MenuDropBg = s.BgSubtle
	p.ContextMenuHeaderFg = s.FgPrimary
	p.ContextMenuHeaderBg = s.BgSubtle
	p.ConfirmDialogBorderFg = s.Warning
	p.SettingsMenuBorderFg = s.BorderAccent
	p.SettingsMenuBg = s.BgSubtle
	p.SettingsMenuTitleFg = s.FgStrong
	p.SettingsMenuTitleBg = s.BgSelectionAlt
	p.SettingsMenuOptionFg = s.FgMuted
	p.SettingsMenuOptionBg = s.BgSubtle
	p.SettingsMenuOptionSelectedFg = s.FgStrong
	p.SettingsMenuOptionSelectedBg = s.BgSelection
	p.SettingsMenuHelpTitleFg = s.Accent
	p.SettingsMenuHelpHintFg = s.FgMuted
	p.SettingsMenuHelpRowFg = s.FgPrimary
	p.SettingsMenuHintFg = s.FgDim
	p.UserBubbleFg = s.FgStrong
	p.UserBubbleBorderFg = s.Border
	p.UserBubbleBg = s.BgSelection
	p.AgentBubbleFg = s.FgPrimary
	p.AgentBubbleBorderFg = s.Border
	p.SystemBubbleBorderFg = s.Border
	p.SystemBubbleFg = s.FgMuted
	p.ReasoningBubbleBorderFg = s.Border
	p.ReasoningBubbleFg = s.FgDim
	p.ApprovalBubbleBorderFg = s.Warning
	p.ApprovalBubbleFg = s.FgStrong
	p.ApprovalResolvedBubbleBorderFg = s.Success
	p.ApprovalResolvedBubbleFg = s.FgPrimary
	p.UserStatusFg = s.FgDim
	p.ChatMetaFg = s.FgDim
	p.ChatMetaSelectedFg = s.Accent
	p.SelectedMessageFg = s.Accent
	p.CopyButtonFg = s.Accent
	p.PinButtonFg = s.Worktree
	p.MoveButtonFg = s.Warning
	p.DeleteButtonFg = s.Danger
	p.ApproveButtonFg = s.Success
	p.DeclineButtonFg = s.Danger
	p.NotesFilterButtonOffFg = s.FgMuted
	p.GuidedWorkflowPromptFrameBorder = s.Workspace
	p.ToastInfoFg = s.FgStrong
	p.ToastInfoBg = s.AccentAlt
	p.ToastWarningFg = s.FgStrong
	p.ToastWarningBg = s.Warning
	p.ToastErrorFg = s.FgStrong
	p.ToastErrorBg = s.Danger
	p.SelectedChatBubbleBorderFg = s.Accent
	p.SidebarSortStripMutedFg = s.SortMuted
	p.SidebarSortStripActiveBg = s.SortActiveBg
	p.MarkdownBlockQuoteFg = s.FgMuted
	p.MarkdownDark = colorAppearsDark(s.BgBase)
	p.ProviderBadgeFallbackFg = s.FgMuted
	p.ProviderBadgeCodexFg = s.Accent
	p.ProviderBadgeClaudeFg = s.Warning
	p.ProviderBadgeOpenCodeFg = s.Workspace
	p.ProviderBadgeKiloCodeFg = s.Worktree
	p.ProviderBadgeGeminiFg = s.AccentAlt
	p.ProviderBadgeCustomFg = s.FgMuted
	p.SettingsMenuOptionFg = s.FgPrimary
	p.SettingsMenuOptionBg = s.BgSubtle
	return p
}

func defaultThemePalette() themePalette {
	return themePalette{
		MainPaneBg:                      "234",
		SidebarPaneBg:                   "235",
		HeaderFg:                        "63",
		StatusFg:                        "245",
		StatusHistorySelectedFg:         "230",
		StatusHistorySelectedBg:         "238",
		ActivityFg:                      "110",
		WorkspaceFg:                     "69",
		WorkspaceActiveFg:               "75",
		WorktreeFg:                      "110",
		WorktreeActiveFg:                "114",
		SessionFg:                       "252",
		SessionUnreadFg:                 "120",
		SelectedFg:                      "230",
		SelectedBg:                      "236",
		MultiSelectFg:                   "230",
		MultiSelectBg:                   "238",
		HighlightRowFg:                  "230",
		HighlightRowBg:                  "240",
		DividerFg:                       "238",
		ScrollbarTrackFg:                "238",
		ScrollbarThumbFg:                "245",
		MenuBarFg:                       "252",
		MenuBarBg:                       "236",
		MenuBarActiveFg:                 "230",
		MenuBarActiveBg:                 "239",
		MenuDropFg:                      "252",
		MenuDropBg:                      "235",
		ContextMenuHeaderFg:             "251",
		ContextMenuHeaderBg:             "235",
		ConfirmDialogBorderFg:           "208",
		SettingsMenuBorderFg:            "63",
		SettingsMenuBg:                  "235",
		SettingsMenuTitleFg:             "230",
		SettingsMenuTitleBg:             "239",
		SettingsMenuOptionFg:            "249",
		SettingsMenuOptionBg:            "235",
		SettingsMenuOptionSelectedFg:    "230",
		SettingsMenuOptionSelectedBg:    "238",
		SettingsMenuHelpTitleFg:         "117",
		SettingsMenuHelpHintFg:          "245",
		SettingsMenuHelpRowFg:           "252",
		SettingsMenuHintFg:              "244",
		UserBubbleFg:                    "230",
		UserBubbleBorderFg:              "240",
		UserBubbleBg:                    "236",
		AgentBubbleFg:                   "252",
		AgentBubbleBorderFg:             "238",
		SystemBubbleBorderFg:            "237",
		SystemBubbleFg:                  "245",
		ReasoningBubbleBorderFg:         "237",
		ReasoningBubbleFg:               "244",
		ApprovalBubbleBorderFg:          "179",
		ApprovalBubbleFg:                "230",
		ApprovalResolvedBubbleBorderFg:  "108",
		ApprovalResolvedBubbleFg:        "251",
		UserStatusFg:                    "243",
		ChatMetaFg:                      "244",
		ChatMetaSelectedFg:              "117",
		SelectedMessageFg:               "117",
		CopyButtonFg:                    "117",
		PinButtonFg:                     "110",
		MoveButtonFg:                    "180",
		DeleteButtonFg:                  "203",
		ApproveButtonFg:                 "70",
		DeclineButtonFg:                 "203",
		NotesFilterButtonOffFg:          "245",
		GuidedWorkflowPromptFrameBorder: "69",
		ToastInfoFg:                     "230",
		ToastInfoBg:                     "29",
		ToastWarningFg:                  "230",
		ToastWarningBg:                  "136",
		ToastErrorFg:                    "230",
		ToastErrorBg:                    "160",
		SelectedChatBubbleBorderFg:      "117",
		SidebarSortStripMutedFg:         "243",
		SidebarSortStripActiveBg:        "60",
		MarkdownBlockQuoteFg:            "245",
		MarkdownDark:                    true,
		ProviderBadgeFallbackFg:         "245",
		ProviderBadgeCodexFg:            "15",
		ProviderBadgeClaudeFg:           "208",
		ProviderBadgeOpenCodeFg:         "39",
		ProviderBadgeKiloCodeFg:         "226",
		ProviderBadgeGeminiFg:           "45",
		ProviderBadgeCustomFg:           "250",
	}
}

func colorAppearsDark(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return true
	}
	if strings.HasPrefix(value, "#") && len(value) == 7 {
		r, errR := strconv.ParseInt(value[1:3], 16, 64)
		g, errG := strconv.ParseInt(value[3:5], 16, 64)
		b, errB := strconv.ParseInt(value[5:7], 16, 64)
		if errR == nil && errG == nil && errB == nil {
			luma := (299*r + 587*g + 114*b) / 1000
			return luma < 128
		}
	}
	return true
}
