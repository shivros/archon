package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

type textInputModeController struct {
	input             *TextInput
	keyString         func(tea.KeyMsg) string
	keyMatchesCommand func(tea.KeyMsg, string, string) bool
	onCancel          func() tea.Cmd
	onSubmit          func(text string) tea.Cmd
	beforeInputUpdate func()
	preHandle         func(key string, msg tea.KeyMsg) (bool, tea.Cmd)
}

func (c textInputModeController) Update(msg tea.Msg) (bool, tea.Cmd) {
	if c.input == nil {
		return true, nil
	}
	keyMsg, isKey := msg.(tea.KeyMsg)
	if !isKey {
		if c.beforeInputUpdate != nil {
			c.beforeInputUpdate()
		}
		return true, c.input.Update(msg)
	}
	key := keyMsg.String()
	if c.keyString != nil {
		key = c.keyString(keyMsg)
	}
	if c.preHandle != nil {
		if handled, cmd := c.preHandle(key, keyMsg); handled {
			return true, cmd
		}
	}
	if c.matchesCommand(keyMsg, KeyCommandInputSelectAll, "ctrl+a") {
		c.input.SelectAll()
		return true, nil
	}
	if c.matchesCommand(keyMsg, KeyCommandInputUndo, "ctrl+z") {
		c.input.Undo()
		return true, nil
	}
	if c.matchesCommand(keyMsg, KeyCommandInputRedo, "ctrl+y") || key == "ctrl+shift+z" {
		c.input.Redo()
		return true, nil
	}
	if c.matchesCommand(keyMsg, KeyCommandInputWordLeft, "ctrl+left") || key == "alt+left" || key == "alt+b" {
		return true, c.input.MoveWordLeft()
	}
	if c.matchesCommand(keyMsg, KeyCommandInputWordRight, "ctrl+right") || key == "alt+right" || key == "alt+f" {
		return true, c.input.MoveWordRight()
	}
	if c.matchesCommand(keyMsg, KeyCommandInputDeleteWordLeft, "alt+backspace") || key == "ctrl+w" {
		return true, c.input.DeleteWordLeft()
	}
	if c.matchesCommand(keyMsg, KeyCommandInputDeleteWordRight, "alt+delete") || key == "alt+d" {
		return true, c.input.DeleteWordRight()
	}
	if c.matchesCommand(keyMsg, KeyCommandInputNewline, "shift+enter") || key == "ctrl+enter" {
		return true, c.input.InsertNewline()
	}
	switch key {
	case "esc":
		if c.onCancel != nil {
			return true, c.onCancel()
		}
		return true, nil
	}
	if c.matchesCommand(keyMsg, KeyCommandInputSubmit, "enter") {
		if c.onSubmit != nil {
			return true, c.onSubmit(strings.TrimSpace(c.input.Value()))
		}
		return true, nil
	}
	if c.beforeInputUpdate != nil {
		c.beforeInputUpdate()
	}
	return true, c.input.Update(msg)
}

func (c textInputModeController) matchesCommand(msg tea.KeyMsg, command, fallback string) bool {
	if c.keyMatchesCommand != nil {
		return c.keyMatchesCommand(msg, command, fallback)
	}
	fallback = strings.TrimSpace(fallback)
	if fallback == "" {
		return false
	}
	key := strings.TrimSpace(msg.String())
	if key == fallback {
		return true
	}
	if c.keyString != nil {
		return strings.TrimSpace(c.keyString(msg)) == fallback
	}
	return false
}
