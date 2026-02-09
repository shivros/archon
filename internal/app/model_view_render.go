package app

import (
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func (m *Model) renderRightPaneView() string {
	headerText, bodyText := m.modeViewContent()
	rightHeader := headerStyle.Render(headerText)
	rightBody := bodyText
	if m.usesViewport() {
		scrollbar := m.viewportScrollbarView()
		if scrollbar != "" {
			rightBody = lipgloss.JoinHorizontal(lipgloss.Top, bodyText, scrollbar)
		}
	}
	inputLine, inputScrollable := m.modeInputView()
	rightLines := []string{rightHeader, rightBody}
	if activity := m.composeActivityLine(time.Now()); activity != "" {
		rightLines = append(rightLines, activityStyle.Render(activity))
	}
	if inputLine != "" {
		dividerWidth := m.viewport.Width
		if dividerWidth <= 0 {
			dividerWidth = max(1, m.width)
		}
		dividerLine := renderInputDivider(dividerWidth, inputScrollable)
		if dividerLine != "" {
			rightLines = append(rightLines, dividerLine)
		}
		rightLines = append(rightLines, inputLine)
	}
	return lipgloss.JoinVertical(lipgloss.Left, rightLines...)
}

func (m *Model) renderBodyWithSidebar(rightView string) string {
	body := rightView
	if m.appState.SidebarCollapsed {
		return body
	}
	listView := ""
	if m.sidebar != nil {
		listView = m.sidebar.View()
	}
	height := max(lipgloss.Height(listView), lipgloss.Height(rightView))
	if height < 1 {
		height = 1
	}
	divider := strings.Repeat("│\n", height-1) + "│"
	return lipgloss.JoinHorizontal(lipgloss.Top, listView, dividerStyle.Render(divider), rightView)
}

func (m *Model) renderStatusLineView() string {
	helpText := ""
	if m.hotkeys != nil {
		helpText = m.hotkeys.Render(m)
	}
	if helpText == "" {
		helpText = "q quit"
	}
	help := helpStyle.Render(helpText)
	status := statusStyle.Render(m.status)
	return renderStatusLine(m.width, help, status)
}

func (m *Model) overlayTransientViews(body string) string {
	menuBar := ""
	if m.menu != nil {
		menuBar = m.menu.MenuBarView(m.width)
	}
	body = overlayLine(body, menuBar, 0)
	if m.menu != nil && m.menu.IsDropdownOpen() {
		menuDrop := m.menu.DropdownView(m.sidebarWidth())
		if menuDrop != "" {
			if m.menu.HasSubmenu() {
				submenu := m.menu.SubmenuView(0)
				combined := combineBlocks(menuDrop, submenu, 1)
				body = overlayBlock(body, combined, 1)
			} else {
				body = overlayBlock(body, menuDrop, 1)
			}
		}
	}
	if m.contextMenu != nil && m.contextMenu.IsOpen() {
		bodyHeight := len(strings.Split(body, "\n"))
		menuBlock, row := m.contextMenu.View(m.width, bodyHeight)
		if menuBlock != "" {
			body = overlayBlock(body, menuBlock, row)
		}
	}
	if m.confirm != nil && m.confirm.IsOpen() {
		bodyHeight := len(strings.Split(body, "\n"))
		confirmBlock, row := m.confirm.View(m.width, bodyHeight)
		if confirmBlock != "" {
			body = overlayBlock(body, confirmBlock, row)
		}
	}
	body = m.overlayToast(body)
	return body
}

func (m *Model) overlayToast(body string) string {
	line := m.toastLine(m.width)
	if line == "" {
		return body
	}
	bodyHeight := len(strings.Split(body, "\n"))
	row := bodyHeight - 1
	if row < 0 {
		return body
	}
	return overlayLine(body, line, row)
}
