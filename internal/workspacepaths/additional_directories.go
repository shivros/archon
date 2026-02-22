package workspacepaths

import (
	"errors"
	"path/filepath"
	"strings"
)

func NormalizeAdditionalDirectories(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(raw))
	seen := map[string]struct{}{}
	for _, value := range raw {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return nil, errors.New("additional directory path is required")
		}
		cleaned := filepath.Clean(trimmed)
		if _, ok := seen[cleaned]; ok {
			continue
		}
		seen[cleaned] = struct{}{}
		out = append(out, cleaned)
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func ResolveAdditionalDirectories(basePath string, directories []string, checker DirChecker) ([]string, error) {
	normalized, err := NormalizeAdditionalDirectories(directories)
	if err != nil {
		return nil, err
	}
	if len(normalized) == 0 {
		return nil, nil
	}
	if checker == nil {
		checker = OSDirChecker()
	}

	baseAbs := ""
	resolveBase := func() (string, error) {
		if baseAbs != "" {
			return baseAbs, nil
		}
		base := strings.TrimSpace(basePath)
		if base == "" {
			return "", errors.New("base path is required to resolve relative additional directories")
		}
		abs, err := filepath.Abs(base)
		if err != nil {
			return "", err
		}
		baseAbs = filepath.Clean(abs)
		return baseAbs, nil
	}

	out := make([]string, 0, len(normalized))
	seen := map[string]struct{}{}
	for _, directory := range normalized {
		resolved := directory
		if !filepath.IsAbs(resolved) {
			base, err := resolveBase()
			if err != nil {
				return nil, err
			}
			resolved = filepath.Join(base, resolved)
		}
		resolved = filepath.Clean(resolved)
		if err := ValidateDirectory(resolved, checker); err != nil {
			return nil, err
		}
		if _, ok := seen[resolved]; ok {
			continue
		}
		seen[resolved] = struct{}{}
		out = append(out, resolved)
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}
