package app

import "charm.land/lipgloss/v2"

const (
	chatBubblePaddingVertical   = 0
	chatBubblePaddingHorizontal = 1
)

const defaultThemeID = "default"

type ThemePreset struct {
	ID      string
	Label   string
	palette themePalette
}

type themePalette struct {
	MainPaneBg                      string
	SidebarPaneBg                   string
	HeaderFg                        string
	StatusFg                        string
	StatusHistorySelectedFg         string
	StatusHistorySelectedBg         string
	ActivityFg                      string
	WorkspaceFg                     string
	WorkspaceActiveFg               string
	WorktreeFg                      string
	WorktreeActiveFg                string
	SessionFg                       string
	SessionUnreadFg                 string
	SelectedFg                      string
	SelectedBg                      string
	MultiSelectFg                   string
	MultiSelectBg                   string
	HighlightRowFg                  string
	HighlightRowBg                  string
	DividerFg                       string
	ScrollbarTrackFg                string
	ScrollbarThumbFg                string
	MenuBarFg                       string
	MenuBarBg                       string
	MenuBarActiveFg                 string
	MenuBarActiveBg                 string
	MenuDropFg                      string
	MenuDropBg                      string
	ContextMenuHeaderFg             string
	ContextMenuHeaderBg             string
	ConfirmDialogBorderFg           string
	SettingsMenuBorderFg            string
	SettingsMenuBg                  string
	SettingsMenuTitleFg             string
	SettingsMenuTitleBg             string
	SettingsMenuOptionFg            string
	SettingsMenuOptionBg            string
	SettingsMenuOptionSelectedFg    string
	SettingsMenuOptionSelectedBg    string
	SettingsMenuHelpTitleFg         string
	SettingsMenuHelpHintFg          string
	SettingsMenuHelpRowFg           string
	SettingsMenuHintFg              string
	UserBubbleFg                    string
	UserBubbleBorderFg              string
	UserBubbleBg                    string
	AgentBubbleFg                   string
	AgentBubbleBorderFg             string
	SystemBubbleBorderFg            string
	SystemBubbleFg                  string
	ReasoningBubbleBorderFg         string
	ReasoningBubbleFg               string
	ApprovalBubbleBorderFg          string
	ApprovalBubbleFg                string
	ApprovalResolvedBubbleBorderFg  string
	ApprovalResolvedBubbleFg        string
	UserStatusFg                    string
	ChatMetaFg                      string
	ChatMetaSelectedFg              string
	SelectedMessageFg               string
	CopyButtonFg                    string
	PinButtonFg                     string
	MoveButtonFg                    string
	DeleteButtonFg                  string
	ApproveButtonFg                 string
	DeclineButtonFg                 string
	NotesFilterButtonOffFg          string
	GuidedWorkflowPromptFrameBorder string
	ToastInfoFg                     string
	ToastInfoBg                     string
	ToastWarningFg                  string
	ToastWarningBg                  string
	ToastErrorFg                    string
	ToastErrorBg                    string
	SelectedChatBubbleBorderFg      string
	SidebarSortStripMutedFg         string
	SidebarSortStripActiveBg        string
	MarkdownBlockQuoteFg            string
	MarkdownDark                    bool
	ProviderBadgeFallbackFg         string
	ProviderBadgeCodexFg            string
	ProviderBadgeClaudeFg           string
	ProviderBadgeOpenCodeFg         string
	ProviderBadgeKiloCodeFg         string
	ProviderBadgeGeminiFg           string
	ProviderBadgeCustomFg           string
}

var (
	mainPaneStyle                   lipgloss.Style
	sidebarPaneStyle                lipgloss.Style
	headerStyle                     lipgloss.Style
	statusStyle                     lipgloss.Style
	statusHistorySelectedStyle      lipgloss.Style
	activityStyle                   lipgloss.Style
	workspaceStyle                  lipgloss.Style
	workspaceActiveStyle            lipgloss.Style
	worktreeStyle                   lipgloss.Style
	worktreeActiveStyle             lipgloss.Style
	sessionStyle                    lipgloss.Style
	sessionUnreadStyle              lipgloss.Style
	selectedStyle                   lipgloss.Style
	multiSelectStyle                lipgloss.Style
	highlightRowStyle               lipgloss.Style
	dividerStyle                    lipgloss.Style
	scrollbarTrackStyle             lipgloss.Style
	scrollbarThumbStyle             lipgloss.Style
	menuBarStyle                    lipgloss.Style
	menuBarActiveStyle              lipgloss.Style
	menuDropStyle                   lipgloss.Style
	contextMenuHeaderStyle          lipgloss.Style
	confirmDialogBorderStyle        lipgloss.Style
	settingsMenuBorderStyle         lipgloss.Style
	settingsMenuTitleStyle          lipgloss.Style
	settingsMenuOptionStyle         lipgloss.Style
	settingsMenuOptionSelectedStyle lipgloss.Style
	settingsMenuHelpTitleStyle      lipgloss.Style
	settingsMenuHelpHintStyle       lipgloss.Style
	settingsMenuHelpRowStyle        lipgloss.Style
	settingsMenuHintStyle           lipgloss.Style
	userBubbleStyle                 lipgloss.Style
	agentBubbleStyle                lipgloss.Style
	systemBubbleStyle               lipgloss.Style
	reasoningBubbleStyle            lipgloss.Style
	approvalBubbleStyle             lipgloss.Style
	approvalResolvedBubbleStyle     lipgloss.Style
	userStatusStyle                 lipgloss.Style
	chatMetaStyle                   lipgloss.Style
	chatMetaSelectedStyle           lipgloss.Style
	selectedMessageStyle            lipgloss.Style
	copyButtonStyle                 lipgloss.Style
	pinButtonStyle                  lipgloss.Style
	moveButtonStyle                 lipgloss.Style
	deleteButtonStyle               lipgloss.Style
	approveButtonStyle              lipgloss.Style
	declineButtonStyle              lipgloss.Style
	notesFilterButtonOffStyle       lipgloss.Style
	guidedWorkflowPromptFrameStyle  lipgloss.Style
	toastInfoStyle                  lipgloss.Style
	toastWarningStyle               lipgloss.Style
	toastErrorStyle                 lipgloss.Style
	selectedChatBubbleBorderColor   string
	sidebarSortStripMutedColor      string
	sidebarSortStripActiveBgColor   string
	markdownBlockQuoteColor         string
	defaultBadgeColor               string
	providerBadgeColors             = map[string]string{}
	activeThemeID                   = defaultThemeID
)

func applyTheme(preset ThemePreset) {
	p := preset.palette
	mainPaneStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(p.SessionFg)).
		Background(lipgloss.Color(p.MainPaneBg))
	sidebarPaneStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(p.SessionFg)).
		Background(lipgloss.Color(p.SidebarPaneBg))
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(p.HeaderFg))
	statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.StatusFg))
	statusHistorySelectedStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(p.StatusHistorySelectedFg)).
		Background(lipgloss.Color(p.StatusHistorySelectedBg)).
		Bold(true)
	activityStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.ActivityFg)).Bold(true)
	workspaceStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.WorkspaceFg)).Bold(true)
	workspaceActiveStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.WorkspaceActiveFg)).Bold(true)
	worktreeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.WorktreeFg))
	worktreeActiveStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.WorktreeActiveFg)).Bold(true)
	sessionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.SessionFg))
	sessionUnreadStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.SessionUnreadFg))
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.SelectedFg)).Background(lipgloss.Color(p.SelectedBg))
	multiSelectStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.MultiSelectFg)).Background(lipgloss.Color(p.MultiSelectBg))
	highlightRowStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.HighlightRowFg)).Background(lipgloss.Color(p.HighlightRowBg))
	dividerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.DividerFg))
	scrollbarTrackStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.ScrollbarTrackFg))
	scrollbarThumbStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.ScrollbarThumbFg))
	menuBarStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.MenuBarFg)).Background(lipgloss.Color(p.MenuBarBg)).Bold(true)
	menuBarActiveStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.MenuBarActiveFg)).Background(lipgloss.Color(p.MenuBarActiveBg)).Bold(true)
	menuDropStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.MenuDropFg)).Background(lipgloss.Color(p.MenuDropBg))
	contextMenuHeaderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.ContextMenuHeaderFg)).Background(lipgloss.Color(p.ContextMenuHeaderBg)).Bold(true)
	confirmDialogBorderStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(p.ConfirmDialogBorderFg))
	settingsMenuBorderStyle = lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(lipgloss.Color(p.SettingsMenuBorderFg)).
		Background(lipgloss.Color(p.SettingsMenuBg)).
		Padding(1, 2)
	settingsMenuTitleStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(p.SettingsMenuTitleFg)).
		Background(lipgloss.Color(p.SettingsMenuTitleBg)).
		Bold(true).
		Padding(0, 1)
	settingsMenuOptionStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(p.SettingsMenuOptionFg)).
		Background(lipgloss.Color(p.SettingsMenuOptionBg))
	settingsMenuOptionSelectedStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(p.SettingsMenuOptionSelectedFg)).
		Background(lipgloss.Color(p.SettingsMenuOptionSelectedBg)).
		Bold(true)
	settingsMenuHelpTitleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.SettingsMenuHelpTitleFg)).Bold(true)
	settingsMenuHelpHintStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.SettingsMenuHelpHintFg))
	settingsMenuHelpRowStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.SettingsMenuHelpRowFg))
	settingsMenuHintStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.SettingsMenuHintFg))
	userBubbleStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Foreground(lipgloss.Color(p.UserBubbleFg)).
		BorderForeground(lipgloss.Color(p.UserBubbleBorderFg)).
		Background(lipgloss.Color(p.UserBubbleBg)).
		Padding(chatBubblePaddingVertical, chatBubblePaddingHorizontal)
	agentBubbleStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Foreground(lipgloss.Color(p.AgentBubbleFg)).
		BorderForeground(lipgloss.Color(p.AgentBubbleBorderFg)).
		Padding(chatBubblePaddingVertical, chatBubblePaddingHorizontal)
	systemBubbleStyle = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(p.SystemBubbleBorderFg)).
		Foreground(lipgloss.Color(p.SystemBubbleFg)).
		Padding(chatBubblePaddingVertical, chatBubblePaddingHorizontal)
	reasoningBubbleStyle = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(p.ReasoningBubbleBorderFg)).
		Foreground(lipgloss.Color(p.ReasoningBubbleFg)).
		Faint(true).
		Padding(chatBubblePaddingVertical, chatBubblePaddingHorizontal)
	approvalBubbleStyle = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(p.ApprovalBubbleBorderFg)).
		Foreground(lipgloss.Color(p.ApprovalBubbleFg)).
		Padding(chatBubblePaddingVertical, chatBubblePaddingHorizontal)
	approvalResolvedBubbleStyle = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(p.ApprovalResolvedBubbleBorderFg)).
		Foreground(lipgloss.Color(p.ApprovalResolvedBubbleFg)).
		Padding(chatBubblePaddingVertical, chatBubblePaddingHorizontal)
	userStatusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.UserStatusFg)).Italic(true)
	chatMetaStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.ChatMetaFg)).Faint(true)
	chatMetaSelectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.ChatMetaSelectedFg)).Bold(true)
	selectedMessageStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.SelectedMessageFg)).Bold(true)
	copyButtonStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.CopyButtonFg)).Bold(true).Underline(true)
	pinButtonStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.PinButtonFg)).Bold(true).Underline(true)
	moveButtonStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.MoveButtonFg)).Bold(true).Underline(true)
	deleteButtonStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.DeleteButtonFg)).Bold(true).Underline(true)
	approveButtonStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.ApproveButtonFg)).Bold(true).Underline(true)
	declineButtonStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.DeclineButtonFg)).Bold(true).Underline(true)
	notesFilterButtonOffStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.NotesFilterButtonOffFg)).Underline(true)
	guidedWorkflowPromptFrameStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(p.GuidedWorkflowPromptFrameBorder)).
		Padding(0, 1)
	toastInfoStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.ToastInfoFg)).Background(lipgloss.Color(p.ToastInfoBg)).Bold(true)
	toastWarningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.ToastWarningFg)).Background(lipgloss.Color(p.ToastWarningBg)).Bold(true)
	toastErrorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.ToastErrorFg)).Background(lipgloss.Color(p.ToastErrorBg)).Bold(true)

	selectedChatBubbleBorderColor = p.SelectedChatBubbleBorderFg
	sidebarSortStripMutedColor = p.SidebarSortStripMutedFg
	sidebarSortStripActiveBgColor = p.SidebarSortStripActiveBg
	markdownBlockQuoteColor = p.MarkdownBlockQuoteFg
	_ = setMarkdownBackgroundDark(p.MarkdownDark)
	defaultBadgeColor = p.ProviderBadgeFallbackFg
	providerBadgeColors = map[string]string{
		"codex":    p.ProviderBadgeCodexFg,
		"claude":   p.ProviderBadgeClaudeFg,
		"opencode": p.ProviderBadgeOpenCodeFg,
		"kilocode": p.ProviderBadgeKiloCodeFg,
		"gemini":   p.ProviderBadgeGeminiFg,
		"custom":   p.ProviderBadgeCustomFg,
	}
	activeThemeID = preset.ID
}
