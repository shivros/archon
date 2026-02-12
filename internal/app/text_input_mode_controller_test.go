package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestTextInputModeControllerShiftEnterInsertsNewline(t *testing.T) {
	input := NewTextInput(40, TextInputConfig{Height: 3})
	input.Focus()
	input.SetValue("hello")

	controller := textInputModeController{
		input: input,
		keyString: func(tea.KeyMsg) string {
			return "shift+enter"
		},
	}

	handled, _ := controller.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !handled {
		t.Fatalf("expected shift+enter to be handled")
	}
	if got := input.Value(); got != "hello\n" {
		t.Fatalf("expected newline insert, got %q", got)
	}
}

func TestTextInputModeControllerEnterCallsSubmitWithTrimmedText(t *testing.T) {
	input := NewTextInput(40, TextInputConfig{Height: 3})
	input.SetValue("  hello world  ")
	called := ""

	controller := textInputModeController{
		input: input,
		keyString: func(tea.KeyMsg) string {
			return "enter"
		},
		onSubmit: func(text string) tea.Cmd {
			called = text
			return nil
		},
	}

	handled, _ := controller.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !handled {
		t.Fatalf("expected enter to be handled")
	}
	if called != "hello world" {
		t.Fatalf("expected trimmed submit value, got %q", called)
	}
}

func TestTextInputModeControllerSupportsRemappedSubmitCommand(t *testing.T) {
	input := NewTextInput(40, TextInputConfig{Height: 3})
	input.SetValue("hello")
	called := false

	controller := textInputModeController{
		input: input,
		keyString: func(tea.KeyMsg) string {
			return "f6"
		},
		keyMatchesCommand: func(msg tea.KeyMsg, command, fallback string) bool {
			return command == KeyCommandInputSubmit && msg.String() == "f6"
		},
		onSubmit: func(text string) tea.Cmd {
			called = text == "hello"
			return nil
		},
	}

	handled, _ := controller.Update(tea.KeyMsg{Type: tea.KeyF6})
	if !handled {
		t.Fatalf("expected remapped submit to be handled")
	}
	if !called {
		t.Fatalf("expected submit handler to be invoked with trimmed value")
	}
}

func TestTextInputSelectAllReplaceUndoRedo(t *testing.T) {
	input := NewTextInput(40, TextInputConfig{Height: 3})
	input.SetValue("hello")
	input.Focus()
	if !input.SelectAll() {
		t.Fatalf("expected select all to succeed")
	}

	_ = input.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if got := input.Value(); got != "x" {
		t.Fatalf("expected replace-all behavior after select all, got %q", got)
	}
	if !input.Undo() {
		t.Fatalf("expected undo to succeed")
	}
	if got := input.Value(); got != "hello" {
		t.Fatalf("expected undo to restore prior value, got %q", got)
	}
	if !input.Redo() {
		t.Fatalf("expected redo to succeed")
	}
	if got := input.Value(); got != "x" {
		t.Fatalf("expected redo to restore replacement value, got %q", got)
	}
}

func TestTextInputModeControllerWordActions(t *testing.T) {
	input := NewTextInput(40, TextInputConfig{Height: 3})
	input.SetValue("hello world")
	input.Focus()

	controller := textInputModeController{
		input: input,
		keyString: func(msg tea.KeyMsg) string {
			return msg.String()
		},
		keyMatchesCommand: func(msg tea.KeyMsg, command, fallback string) bool {
			switch command {
			case KeyCommandInputWordLeft:
				return msg.String() == "f7"
			case KeyCommandInputDeleteWordLeft:
				return msg.String() == "f8"
			default:
				return false
			}
		},
	}

	handled, _ := controller.Update(tea.KeyMsg{Type: tea.KeyF7})
	if !handled {
		t.Fatalf("expected remapped word-left action to be handled")
	}
	handled, _ = controller.Update(tea.KeyMsg{Type: tea.KeyF8})
	if !handled {
		t.Fatalf("expected remapped delete-word-left action to be handled")
	}
	if got := input.Value(); got != "world" {
		t.Fatalf("expected delete-word-left after move to keep trailing word, got %q", got)
	}
}

func TestInputPanelComposesFooterWithBaseInput(t *testing.T) {
	input := NewTextInput(40, TextInputConfig{Height: 1})
	input.SetValue("hello")

	line, _ := InputPanel{
		Input: input,
		Footer: InputFooterFunc(func() string {
			return "Model: gpt-5"
		}),
	}.View()
	if !strings.Contains(line, "hello") {
		t.Fatalf("expected base input line in view, got %q", line)
	}
	if !strings.Contains(line, "Model: gpt-5") {
		t.Fatalf("expected composed footer in view, got %q", line)
	}
}

func TestNewTextInputAddsCtrlWordNavigationAliases(t *testing.T) {
	input := NewTextInput(40, TextInputConfig{Height: 1})
	if input == nil {
		t.Fatalf("expected input")
	}
	assertHasKeyBinding(t, input.input.KeyMap.WordBackward.Keys(), "ctrl+left")
	assertHasKeyBinding(t, input.input.KeyMap.WordForward.Keys(), "ctrl+right")
	assertHasKeyBinding(t, input.input.KeyMap.DeleteWordBackward.Keys(), "ctrl+backspace")
	assertHasKeyBinding(t, input.input.KeyMap.DeleteWordForward.Keys(), "ctrl+delete")
}

func assertHasKeyBinding(t *testing.T, keys []string, want string) {
	t.Helper()
	for _, key := range keys {
		if key == want {
			return
		}
	}
	t.Fatalf("expected key binding %q in %v", want, keys)
}
