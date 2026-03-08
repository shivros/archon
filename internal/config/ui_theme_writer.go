package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func updateUIThemeAtPath(path string, themeName string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("path is required")
	}
	theme := normalizeUIThemeName(themeName)
	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		data = nil
	}
	next := updateUIThemeDocument(string(data), theme)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if !strings.HasSuffix(next, "\n") {
		next += "\n"
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(next), 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func updateUIThemeDocument(doc, theme string) string {
	theme = normalizeUIThemeName(theme)
	if strings.TrimSpace(doc) == "" {
		return renderThemeSection(theme)
	}
	lines := strings.Split(doc, "\n")
	start, end := themeSectionBounds(lines)
	if start < 0 {
		return appendThemeSection(lines, theme)
	}
	if idx := themeNameLineIndex(lines, start+1, end); idx >= 0 {
		lines[idx] = withThemeName(lines[idx], theme)
		return strings.Join(lines, "\n")
	}
	insertAt := start + 1
	for insertAt < end {
		trimmed := strings.TrimSpace(strings.TrimSuffix(lines[insertAt], "\r"))
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			insertAt++
			continue
		}
		break
	}
	lines = insertLine(lines, insertAt, `name = "`+theme+`"`)
	return strings.Join(lines, "\n")
}

func renderThemeSection(theme string) string {
	return "[theme]\nname = \"" + theme + "\"\n"
}

func appendThemeSection(lines []string, theme string) string {
	doc := strings.Join(lines, "\n")
	doc = strings.TrimRight(doc, "\n")
	if strings.TrimSpace(doc) == "" {
		return renderThemeSection(theme)
	}
	if strings.TrimSpace(lastLine(doc)) != "" {
		doc += "\n"
	}
	doc += "\n[theme]\nname = \"" + theme + "\""
	return doc
}

func themeSectionBounds(lines []string) (int, int) {
	start := -1
	end := len(lines)
	for idx, line := range lines {
		name, ok := parseSectionName(line)
		if !ok {
			continue
		}
		if start >= 0 {
			end = idx
			break
		}
		if name == "theme" {
			start = idx
		}
	}
	return start, end
}

func parseSectionName(line string) (string, bool) {
	trimmed := strings.TrimSpace(strings.TrimSuffix(line, "\r"))
	if !strings.HasPrefix(trimmed, "[") {
		return "", false
	}
	end := strings.Index(trimmed, "]")
	if end <= 1 {
		return "", false
	}
	name := strings.TrimSpace(trimmed[1:end])
	if name == "" {
		return "", false
	}
	return strings.ToLower(name), true
}

func themeNameLineIndex(lines []string, start, end int) int {
	if start < 0 {
		start = 0
	}
	if end > len(lines) {
		end = len(lines)
	}
	for idx := start; idx < end; idx++ {
		trimmed := strings.TrimSpace(strings.TrimSuffix(lines[idx], "\r"))
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		head, _ := splitInlineComment(lines[idx])
		eq := strings.Index(head, "=")
		if eq <= 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(head[:eq]))
		if key == "name" {
			return idx
		}
	}
	return -1
}

func withThemeName(line, theme string) string {
	indent := leadingWhitespace(line)
	_, comment := splitInlineComment(line)
	out := indent + `name = "` + normalizeUIThemeName(theme) + `"`
	if comment != "" {
		out += " " + comment
	}
	if strings.HasSuffix(line, "\r") {
		out += "\r"
	}
	return out
}

func splitInlineComment(line string) (string, string) {
	inString := false
	escaped := false
	for idx, r := range line {
		switch {
		case escaped:
			escaped = false
		case r == '\\' && inString:
			escaped = true
		case r == '"':
			inString = !inString
		case r == '#' && !inString:
			body := strings.TrimRight(line[:idx], " \t")
			return body, strings.TrimSpace(line[idx:])
		}
	}
	return strings.TrimRight(line, " \t"), ""
}

func leadingWhitespace(line string) string {
	for i, r := range line {
		if r != ' ' && r != '\t' {
			return line[:i]
		}
	}
	return line
}

func insertLine(lines []string, at int, line string) []string {
	if at < 0 {
		at = 0
	}
	if at > len(lines) {
		at = len(lines)
	}
	lines = append(lines, "")
	copy(lines[at+1:], lines[at:])
	lines[at] = line
	return lines
}

func lastLine(doc string) string {
	if idx := strings.LastIndex(doc, "\n"); idx >= 0 {
		return doc[idx+1:]
	}
	return doc
}
