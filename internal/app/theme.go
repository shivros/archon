package app

import "charm.land/lipgloss/v2"

const (
	chatBubblePaddingVertical   = 0
	chatBubblePaddingHorizontal = 1
)

var (
	headerStyle                    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	helpStyle                      = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	statusStyle                    = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	activityStyle                  = lipgloss.NewStyle().Foreground(lipgloss.Color("110")).Bold(true)
	workspaceStyle                 = lipgloss.NewStyle().Foreground(lipgloss.Color("69")).Bold(true)
	workspaceActiveStyle           = lipgloss.NewStyle().Foreground(lipgloss.Color("75")).Bold(true)
	worktreeStyle                  = lipgloss.NewStyle().Foreground(lipgloss.Color("110"))
	worktreeActiveStyle            = lipgloss.NewStyle().Foreground(lipgloss.Color("114")).Bold(true)
	sessionStyle                   = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	sessionUnreadStyle             = lipgloss.NewStyle().Foreground(lipgloss.Color("120"))
	activeSessionStyle             = lipgloss.NewStyle().Foreground(lipgloss.Color("70"))
	selectedStyle                  = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("236"))
	dividerStyle                   = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	scrollbarTrackStyle            = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	scrollbarThumbStyle            = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	menuBarStyle                   = lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Background(lipgloss.Color("236")).Bold(true)
	menuBarActiveStyle             = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("239")).Bold(true)
	menuDropStyle                  = lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Background(lipgloss.Color("235"))
	contextMenuHeaderStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("251")).Background(lipgloss.Color("235")).Bold(true)
	confirmDialogBorderStyle       = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("208"))
	userBubbleStyle                = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Background(lipgloss.Color("236")).Padding(chatBubblePaddingVertical, chatBubblePaddingHorizontal)
	agentBubbleStyle               = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("238")).Padding(chatBubblePaddingVertical, chatBubblePaddingHorizontal)
	systemBubbleStyle              = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("237")).Foreground(lipgloss.Color("245")).Padding(chatBubblePaddingVertical, chatBubblePaddingHorizontal)
	reasoningBubbleStyle           = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("237")).Foreground(lipgloss.Color("244")).Faint(true).Padding(chatBubblePaddingVertical, chatBubblePaddingHorizontal)
	approvalBubbleStyle            = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("179")).Foreground(lipgloss.Color("230")).Padding(chatBubblePaddingVertical, chatBubblePaddingHorizontal)
	approvalResolvedBubbleStyle    = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("108")).Foreground(lipgloss.Color("251")).Padding(chatBubblePaddingVertical, chatBubblePaddingHorizontal)
	userStatusStyle                = lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Italic(true)
	chatMetaStyle                  = lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Faint(true)
	chatMetaSelectedStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("117")).Bold(true)
	selectedMessageStyle           = lipgloss.NewStyle().Foreground(lipgloss.Color("117")).Bold(true)
	copyButtonStyle                = lipgloss.NewStyle().Foreground(lipgloss.Color("117")).Bold(true).Underline(true)
	pinButtonStyle                 = lipgloss.NewStyle().Foreground(lipgloss.Color("110")).Bold(true).Underline(true)
	moveButtonStyle                = lipgloss.NewStyle().Foreground(lipgloss.Color("180")).Bold(true).Underline(true)
	deleteButtonStyle              = lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true).Underline(true)
	approveButtonStyle             = lipgloss.NewStyle().Foreground(lipgloss.Color("70")).Bold(true).Underline(true)
	declineButtonStyle             = lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true).Underline(true)
	notesFilterButtonOffStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Underline(true)
	guidedWorkflowPromptFrameStyle = lipgloss.NewStyle().
					Border(lipgloss.RoundedBorder()).
					BorderForeground(lipgloss.Color("69")).
					Padding(0, 1)
	toastInfoStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("29")).Bold(true)
	toastWarningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("136")).Bold(true)
	toastErrorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("160")).Bold(true)
)
