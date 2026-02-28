package app

import tea "charm.land/bubbletea/v2"

// KeyResolver provides keyboard input resolution for controllers that need
// to interpret key events through user-configured remappings. Model implements
// this interface; test stubs can embed a minimal implementation.
type KeyResolver interface {
	keyString(msg tea.KeyMsg) string
	keyMatchesCommand(msg tea.KeyMsg, command, fallback string) bool
}
