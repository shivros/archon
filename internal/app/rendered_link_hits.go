package app

import (
	"strings"

	xansi "github.com/charmbracelet/x/ansi"
)

type markdownInlineLink struct {
	Label  string
	Target string
}

func extractMarkdownInlineLinks(input string) []markdownInlineLink {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}
	links := make([]markdownInlineLink, 0, 4)
	for i := 0; i < len(input); i++ {
		if input[i] != '[' {
			continue
		}
		if i > 0 && input[i-1] == '!' {
			continue
		}
		closeLabel := strings.IndexByte(input[i+1:], ']')
		if closeLabel < 0 {
			continue
		}
		closeLabel += i + 1
		if closeLabel+1 >= len(input) || input[closeLabel+1] != '(' {
			continue
		}
		targetStart := closeLabel + 2
		depth := 1
		targetEnd := -1
	loop:
		for j := targetStart; j < len(input); j++ {
			switch input[j] {
			case '\\':
				j++
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 {
					targetEnd = j
					break loop
				}
			}
		}
		if targetEnd < 0 {
			continue
		}
		label := strings.TrimSpace(input[i+1 : closeLabel])
		target := strings.TrimSpace(input[targetStart:targetEnd])
		if strings.HasPrefix(target, "<") && strings.HasSuffix(target, ">") && len(target) >= 2 {
			target = strings.TrimSpace(target[1 : len(target)-1])
		}
		if label != "" && target != "" {
			links = append(links, markdownInlineLink{Label: label, Target: target})
		}
		i = targetEnd
	}
	if len(links) == 0 {
		return nil
	}
	return links
}

func buildRenderedLinkHits(markdown string, plainLines []string, lineOffset int) []renderedLinkHit {
	if lineOffset < 0 || len(plainLines) == 0 {
		return nil
	}
	links := extractMarkdownInlineLinks(markdown)
	if len(links) == 0 {
		return nil
	}
	hits := make([]renderedLinkHit, 0, len(links))
	searchLine := 0
	searchCol := 0
	for _, link := range links {
		line, col, ok := findRenderedLabelPosition(plainLines, link.Label, searchLine, searchCol)
		if !ok {
			line, col, ok = findRenderedLabelPosition(plainLines, link.Label, 0, 0)
		}
		if !ok {
			continue
		}
		start := xansi.StringWidth(plainLines[line][:col])
		end := start + xansi.StringWidth(link.Label) - 1
		if end < start {
			continue
		}
		hits = append(hits, renderedLinkHit{
			Label:  link.Label,
			Target: link.Target,
			Line:   lineOffset + line,
			Start:  start,
			End:    end,
		})
		searchLine = line
		searchCol = col + len(link.Label)
		if searchCol >= len(plainLines[line]) {
			searchLine++
			searchCol = 0
		}
	}
	if len(hits) == 0 {
		return nil
	}
	return hits
}

func findRenderedLabelPosition(lines []string, label string, startLine int, startCol int) (int, int, bool) {
	label = strings.TrimSpace(label)
	if label == "" || len(lines) == 0 {
		return 0, 0, false
	}
	if startLine < 0 {
		startLine = 0
	}
	for line := startLine; line < len(lines); line++ {
		text := lines[line]
		col := 0
		if line == startLine {
			col = max(0, min(startCol, len(text)))
		}
		if idx := strings.Index(text[col:], label); idx >= 0 {
			return line, col + idx, true
		}
	}
	return 0, 0, false
}
