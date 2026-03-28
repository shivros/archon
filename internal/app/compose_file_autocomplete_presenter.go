package app

import (
	"strings"

	"control/internal/types"
)

func composeFileSearchPickerOptions(
	candidateIDs []string,
	candidates map[string]types.FileSearchCandidate,
	loading bool,
) []selectOption {
	options := make([]selectOption, 0, max(1, len(candidateIDs)))
	for _, id := range candidateIDs {
		candidate, ok := candidates[id]
		if !ok {
			continue
		}
		label := strings.TrimSpace(candidate.DisplayPath)
		if label == "" {
			label = strings.TrimSpace(candidate.Path)
		}
		if label == "" {
			continue
		}
		options = append(options, selectOption{
			id:     strings.TrimSpace(candidate.Path),
			label:  label,
			search: label + " " + strings.TrimSpace(candidate.Path),
		})
	}
	if len(options) > 0 {
		return options
	}
	label := " (no files found)"
	if loading {
		label = " (loading files...)"
	}
	return []selectOption{{label: label}}
}

func composeFileSearchSupplementalView(loading bool, candidateCount int) string {
	if loading && candidateCount > 0 {
		return " loading files..."
	}
	return ""
}
