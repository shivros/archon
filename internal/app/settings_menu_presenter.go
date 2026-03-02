package app

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"
)

type SettingsMenuPresenter interface {
	View(*SettingsMenuController, int, int, []SettingsHotkeyMapping) (string, int)
}

type defaultSettingsMenuPresenter struct{}

func (defaultSettingsMenuPresenter) View(controller *SettingsMenuController, maxWidth, maxHeight int, mappings []SettingsHotkeyMapping) (string, int) {
	if controller == nil || !controller.open {
		return "", 0
	}
	if controller.screen == settingsMenuScreenHelp {
		return settingsMenuHelpView(controller, maxWidth, maxHeight, mappings)
	}
	return settingsMenuRootView(controller, maxWidth, maxHeight)
}

func settingsMenuRootView(controller *SettingsMenuController, maxWidth, maxHeight int) (string, int) {
	lines := []string{settingsMenuTitleStyle.Render(" SETTINGS "), ""}
	for idx, item := range controller.items {
		word := settingsMenuWordArt(item.Title)
		if idx == controller.selected {
			lines = append(lines, settingsMenuOptionSelectedStyle.Render(word))
		} else {
			lines = append(lines, settingsMenuOptionStyle.Render(word))
		}
		if idx < len(controller.items)-1 {
			lines = append(lines, "")
		}
	}
	lines = append(lines, "", settingsMenuHintStyle.Render("enter select  j/k move  esc close"))
	block := settingsMenuBorderStyle.Render(strings.Join(lines, "\n"))
	return centerOverlayBlock(block, maxWidth, maxHeight)
}

func settingsMenuHelpView(controller *SettingsMenuController, maxWidth, maxHeight int, mappings []SettingsHotkeyMapping) (string, int) {
	if maxWidth <= 0 {
		maxWidth = 80
	}
	if maxHeight <= 0 {
		maxHeight = 12
	}
	innerWidth := max(32, min(maxWidth-8, 96))
	rowWidth := max(1, innerWidth-4)

	rows := make([]string, 0, len(mappings)+3)
	rows = append(rows, settingsMenuHelpTitleStyle.Render(padToWidth("HOTKEY HELP", rowWidth)))
	rows = append(rows, settingsMenuHelpHintStyle.Render(padToWidth("all active mappings", rowWidth)))
	rows = append(rows, "")
	for _, item := range mappings {
		context := truncateToWidth(item.Context, 16)
		key := truncateToWidth(item.Key, 20)
		label := truncateToWidth(item.Label, max(1, rowWidth-16-2-20-2))
		line := fmt.Sprintf("%s  %s  %s", padToWidth(context, 16), padToWidth(key, 20), label)
		rows = append(rows, settingsMenuHelpRowStyle.Render(truncateToWidth(line, rowWidth)))
	}
	if len(mappings) == 0 {
		rows = append(rows, settingsMenuHelpRowStyle.Render(padToWidth("no hotkeys available", rowWidth)))
	}

	visibleRows := max(6, maxHeight-8)
	maxOffset := max(0, len(rows)-visibleRows)
	if controller.helpOffset > maxOffset {
		controller.helpOffset = maxOffset
	}
	if controller.helpOffset < 0 {
		controller.helpOffset = 0
	}

	start := controller.helpOffset
	end := min(len(rows), start+visibleRows)
	visible := rows[start:end]
	footer := settingsMenuHintStyle.Render(fmt.Sprintf("rows %d-%d/%d  j/k scroll  esc back", start+1, end, len(rows)))
	content := strings.Join(append(visible, "", footer), "\n")
	block := settingsMenuBorderStyle.Render(content)
	return centerOverlayBlock(block, maxWidth, maxHeight)
}

func settingsMenuWordArt(word string) string {
	switch strings.ToUpper(strings.TrimSpace(word)) {
	case "HELP":
		return strings.Join([]string{
			" _   _ _____ _     ____  ",
			"| | | | ____| |   |  _ \\ ",
			"| |_| |  _| | |   | |_) |",
			"|  _  | |___| |___|  __/ ",
			"|_| |_|_____|_____|_|    ",
		}, "\n")
	case "QUIT":
		return strings.Join([]string{
			"  ___  _   _ ___ _____ ",
			" / _ \\| | | |_ _|_   _|",
			"| | | | | | || |  | |  ",
			"| |_| | |_| || |  | |  ",
			" \\__\\_\\___/|___| |_|  ",
		}, "\n")
	default:
		return strings.ToUpper(word)
	}
}

func centerOverlayBlock(block string, maxWidth, maxHeight int) (string, int) {
	block = strings.TrimRight(block, "\n")
	if block == "" {
		return "", 0
	}
	width := xansi.StringWidth(longestLine(block))
	height := lipgloss.Height(block)
	if width <= 0 {
		width = 1
	}
	if height <= 0 {
		height = 1
	}
	x := 0
	y := 0
	if maxWidth > width {
		x = (maxWidth - width) / 2
	}
	if maxHeight > height {
		y = (maxHeight - height) / 2
		if y < 1 && maxHeight > 2 {
			y = 1
		}
	}
	if x > 0 {
		block = indentBlock(block, x)
	}
	return block, y
}

func longestLine(block string) string {
	lines := strings.Split(block, "\n")
	best := ""
	bestWidth := -1
	for _, line := range lines {
		if w := xansi.StringWidth(line); w > bestWidth {
			bestWidth = w
			best = line
		}
	}
	return best
}
