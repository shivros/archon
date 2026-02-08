package app

import (
	"strings"

	xansi "github.com/charmbracelet/x/ansi"
)

func padLines(lines []string, width int) string {
	if width <= 0 {
		return strings.Join(lines, "\n")
	}
	out := make([]string, len(lines))
	for i, line := range lines {
		lineWidth := xansi.StringWidth(line)
		if lineWidth < width {
			line = line + strings.Repeat(" ", width-lineWidth)
		}
		out[i] = line
	}
	return strings.Join(out, "\n")
}
