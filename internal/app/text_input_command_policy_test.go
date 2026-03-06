package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestDefaultTextInputCommandPolicyNewlineCommandMatchWins(t *testing.T) {
	policy := defaultTextInputCommandPolicy{}
	msg := tea.KeyPressMsg{Code: tea.KeyEnter}
	if !policy.IsInputNewline(msg, "enter", true) {
		t.Fatalf("expected matched newline command to win")
	}
}

func TestDefaultTextInputCommandPolicyUsesCompatibilityAliases(t *testing.T) {
	policy := defaultTextInputCommandPolicy{}

	if !policy.IsInputNewline(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModCtrl}, "ctrl+enter", false) {
		t.Fatalf("expected ctrl+enter compatibility alias to insert newline")
	}
	if !policy.IsInputNewline(tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl}, "ctrl+j", false) {
		t.Fatalf("expected ctrl+j compatibility alias to insert newline")
	}
	if !policy.IsInputNewline(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift}, "enter", false) {
		t.Fatalf("expected shift+enter modifier fallback to insert newline")
	}
}

func TestDefaultTextInputCommandPolicyIgnoresPlainEnterWhenUnmatched(t *testing.T) {
	policy := defaultTextInputCommandPolicy{}
	if policy.IsInputNewline(tea.KeyPressMsg{Code: tea.KeyEnter}, "enter", false) {
		t.Fatalf("expected plain enter to not resolve as newline when command is unmatched")
	}
}
