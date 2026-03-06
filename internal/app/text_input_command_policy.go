package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// textInputCommandPolicy resolves command semantics that can vary across
// environments while keeping controller flow stable.
type textInputCommandPolicy interface {
	IsInputNewline(msg tea.KeyMsg, resolvedKey string, newlineCommandMatched bool) bool
}

type defaultTextInputCommandPolicy struct{}

func (defaultTextInputCommandPolicy) IsInputNewline(msg tea.KeyMsg, resolvedKey string, newlineCommandMatched bool) bool {
	if newlineCommandMatched {
		return true
	}
	return isCompatibilityInputNewlineKey(msg, resolvedKey)
}

func isCompatibilityInputNewlineKey(msg tea.KeyMsg, resolvedKey string) bool {
	switch strings.TrimSpace(resolvedKey) {
	case "ctrl+enter", "ctrl+j":
		return true
	}
	return isShiftOrCtrlEnterKey(msg)
}
