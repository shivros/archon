package app

import "github.com/charmbracelet/lipgloss"

var (
	headerStyle          = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	helpStyle            = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	statusStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	workspaceStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("69")).Bold(true)
	workspaceActiveStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("75")).Bold(true)
	worktreeStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("110"))
	worktreeActiveStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("114")).Bold(true)
	sessionStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	activeSessionStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("70"))
	sessionSelectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Background(lipgloss.Color("238"))
	selectedStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("236"))
	dividerStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	scrollbarTrackStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	scrollbarThumbStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
)
