package daemon

import (
	"fmt"
	"path/filepath"
	"strings"

	"control/internal/providers"
)

const geminiMaxIncludeDirectories = 5

type additionalDirectoryAdapter interface {
	BuildArgs(directories []string) ([]string, error)
	BuildPermission(directories []string) (*openCodePermissionCreateRequest, error)
}

type additionalDirectoryNoopAdapter struct{}

func (additionalDirectoryNoopAdapter) BuildArgs(_ []string) ([]string, error) {
	return nil, nil
}

func (additionalDirectoryNoopAdapter) BuildPermission(_ []string) (*openCodePermissionCreateRequest, error) {
	return nil, nil
}

type additionalDirectoryFlagAdapter struct {
	flag           string
	providerName   string
	maxDirectories int
}

func (a additionalDirectoryFlagAdapter) BuildArgs(directories []string) ([]string, error) {
	if a.maxDirectories > 0 && len(directories) > a.maxDirectories {
		return nil, fmt.Errorf("%s supports up to %d include directories", a.providerName, a.maxDirectories)
	}
	out := make([]string, 0, len(directories)*2)
	for _, directory := range directories {
		trimmed := strings.TrimSpace(directory)
		if trimmed == "" {
			continue
		}
		out = append(out, a.flag, trimmed)
	}
	return out, nil
}

func (additionalDirectoryFlagAdapter) BuildPermission(_ []string) (*openCodePermissionCreateRequest, error) {
	return nil, nil
}

type additionalDirectoryOpenCodeAdapter struct{}

func (additionalDirectoryOpenCodeAdapter) BuildArgs(_ []string) ([]string, error) {
	return nil, nil
}

func (additionalDirectoryOpenCodeAdapter) BuildPermission(directories []string) (*openCodePermissionCreateRequest, error) {
	patterns := openCodeExternalDirectoryPatterns(directories)
	if len(patterns) == 0 {
		return nil, nil
	}
	return &openCodePermissionCreateRequest{
		Permission: "external_directory",
		Patterns:   patterns,
	}, nil
}

var additionalDirectoryAdapters = map[string]additionalDirectoryAdapter{
	"codex": additionalDirectoryFlagAdapter{
		flag:         "--add-dir",
		providerName: "codex",
	},
	"claude": additionalDirectoryFlagAdapter{
		flag:         "--add-dir",
		providerName: "claude",
	},
	"gemini": additionalDirectoryFlagAdapter{
		flag:           "--include-directories",
		providerName:   "gemini",
		maxDirectories: geminiMaxIncludeDirectories,
	},
	"opencode": additionalDirectoryOpenCodeAdapter{},
	"kilocode": additionalDirectoryOpenCodeAdapter{},
}

func additionalDirectoryAdapterForProvider(provider string) additionalDirectoryAdapter {
	adapter, ok := additionalDirectoryAdapters[providers.Normalize(provider)]
	if !ok {
		return additionalDirectoryNoopAdapter{}
	}
	return adapter
}

func providerAdditionalDirectoryArgs(provider string, directories []string) ([]string, error) {
	return additionalDirectoryAdapterForProvider(provider).BuildArgs(directories)
}

func providerAdditionalDirectoryPermission(provider string, directories []string) (*openCodePermissionCreateRequest, error) {
	return additionalDirectoryAdapterForProvider(provider).BuildPermission(directories)
}

func openCodeExternalDirectoryPatterns(directories []string) []string {
	if len(directories) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(directories))
	out := make([]string, 0, len(directories))
	for _, directory := range directories {
		trimmed := strings.TrimSpace(directory)
		if trimmed == "" {
			continue
		}
		cleaned := filepath.Clean(trimmed)
		if cleaned == "." {
			continue
		}
		pattern := cleaned
		if !strings.HasSuffix(pattern, string(filepath.Separator)) {
			pattern += string(filepath.Separator)
		}
		pattern += "*"
		if _, ok := seen[pattern]; ok {
			continue
		}
		seen[pattern] = struct{}{}
		out = append(out, pattern)
	}
	return out
}
