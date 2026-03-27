package app

import (
	"strings"
	"unicode"

	"control/internal/types"
)

type composeFileSearchFragment struct {
	Start  int
	End    int
	Cursor int
	Query  string
}

func activeComposeFileSearchFragment(value string, cursor int) (composeFileSearchFragment, bool) {
	runes := []rune(value)
	cursor = clamp(cursor, 0, len(runes))
	start := -1
	for i := cursor - 1; i >= 0; i-- {
		if runes[i] == '@' {
			start = i
			break
		}
		if isComposeFileSearchBoundary(runes[i]) {
			break
		}
	}
	if start < 0 {
		return composeFileSearchFragment{}, false
	}
	if start > 0 && !isComposeFileSearchTriggerBoundary(runes[start-1]) {
		return composeFileSearchFragment{}, false
	}
	end := cursor
	for end < len(runes) && !isComposeFileSearchBoundary(runes[end]) {
		end++
	}
	return composeFileSearchFragment{
		Start:  start,
		End:    end,
		Cursor: cursor,
		Query:  string(runes[start+1 : cursor]),
	}, true
}

func isComposeFileSearchTriggerBoundary(r rune) bool {
	return unicode.IsSpace(r) || strings.ContainsRune("([{<\"'`", r)
}

func isComposeFileSearchBoundary(r rune) bool {
	if unicode.IsSpace(r) {
		return true
	}
	return strings.ContainsRune(",;:!?()[]{}<>\"'`", r)
}

func composeFileSearchMentionText(candidate types.FileSearchCandidate) string {
	display := strings.TrimSpace(candidate.DisplayPath)
	if display == "" {
		display = strings.TrimSpace(candidate.Path)
	}
	if display == "" {
		return ""
	}
	return "@" + display
}

func replaceComposeFileSearchFragment(input *TextInput, fragment composeFileSearchFragment, candidate types.FileSearchCandidate) bool {
	if input == nil {
		return false
	}
	mention := composeFileSearchMentionText(candidate)
	if mention == "" {
		return false
	}
	replacement := mention
	valueRunes := []rune(input.Value())
	if fragment.End >= len(valueRunes) || !unicode.IsSpace(valueRunes[fragment.End]) {
		replacement += " "
	}
	return input.ReplaceRuneRange(fragment.Start, fragment.End, replacement)
}
