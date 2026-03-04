package app

import (
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var (
	pathLinePattern     = regexp.MustCompile(`^(/.+?):(\d+)(?::(\d+))?$`)
	fragmentLinePattern = regexp.MustCompile(`^L?(\d+)(?:C(\d+))?$`)
)

type defaultFileLinkResolver struct{}

func (defaultFileLinkResolver) Resolve(rawTarget string) (ResolvedFileLink, error) {
	rawTarget = strings.TrimSpace(rawTarget)
	if rawTarget == "" {
		return ResolvedFileLink{}, errFileLinkEmptyTarget
	}
	resolved := ResolvedFileLink{RawTarget: rawTarget}

	target := rawTarget
	fragment := ""

	if strings.Contains(target, "://") {
		parsed, err := url.Parse(target)
		if err != nil {
			return ResolvedFileLink{}, fmt.Errorf("parse link: %w", err)
		}
		if !strings.EqualFold(strings.TrimSpace(parsed.Scheme), "file") {
			return ResolvedFileLink{}, fmt.Errorf("%w: %s", errFileLinkUnsupportedTarget, strings.TrimSpace(parsed.Scheme))
		}
		target = parsed.Path
		fragment = parsed.Fragment
	} else {
		if idx := strings.IndexByte(target, '#'); idx >= 0 {
			fragment = target[idx+1:]
			target = target[:idx]
		}
		if idx := strings.IndexByte(target, '?'); idx >= 0 {
			target = target[:idx]
		}
	}

	target = strings.TrimSpace(target)
	if target == "" {
		return ResolvedFileLink{}, errFileLinkEmptyTarget
	}
	if unescaped, err := url.PathUnescape(target); err == nil {
		target = unescaped
	}

	line, column := parseFileLinkFragment(fragment)
	if base, suffixLine, suffixColumn, ok := splitFileLinkLineSuffix(target); ok {
		target = base
		if line <= 0 {
			line = suffixLine
			column = suffixColumn
		}
	}

	if !filepath.IsAbs(target) {
		return ResolvedFileLink{}, fmt.Errorf("%w: %s", errFileLinkUnsupportedTarget, rawTarget)
	}

	resolved.Path = filepath.Clean(target)
	resolved.Line = max(0, line)
	resolved.Column = max(0, column)
	return resolved, nil
}

func splitFileLinkLineSuffix(path string) (string, int, int, bool) {
	matches := pathLinePattern.FindStringSubmatch(strings.TrimSpace(path))
	if len(matches) == 0 {
		return "", 0, 0, false
	}
	line, err := strconv.Atoi(matches[2])
	if err != nil || line <= 0 {
		return "", 0, 0, false
	}
	column := 0
	if len(matches) >= 4 && strings.TrimSpace(matches[3]) != "" {
		col, err := strconv.Atoi(matches[3])
		if err == nil && col > 0 {
			column = col
		}
	}
	return strings.TrimSpace(matches[1]), line, column, true
}

func parseFileLinkFragment(fragment string) (int, int) {
	fragment = strings.TrimSpace(fragment)
	if fragment == "" {
		return 0, 0
	}
	matches := fragmentLinePattern.FindStringSubmatch(fragment)
	if len(matches) == 0 {
		return 0, 0
	}
	line, _ := strconv.Atoi(matches[1])
	if line <= 0 {
		return 0, 0
	}
	column := 0
	if len(matches) >= 3 && strings.TrimSpace(matches[2]) != "" {
		column, _ = strconv.Atoi(matches[2])
		if column < 0 {
			column = 0
		}
	}
	return line, column
}
