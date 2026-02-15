package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
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

	handled, _ := controller.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
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

	handled, _ := controller.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
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

	handled, _ := controller.Update(tea.KeyPressMsg{Code: tea.KeyF6})
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

	_ = input.Update(tea.KeyPressMsg{Text: "x"})
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

	handled, _ := controller.Update(tea.KeyPressMsg{Code: tea.KeyF7})
	if !handled {
		t.Fatalf("expected remapped word-left action to be handled")
	}
	handled, _ = controller.Update(tea.KeyPressMsg{Code: tea.KeyF8})
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
	assertHasKeyBinding(t, input.input.KeyMap.DeleteWordBackward.Keys(), "alt+backspace")
	assertHasKeyBinding(t, input.input.KeyMap.DeleteWordForward.Keys(), "alt+delete")
}

func TestSingleLineTextInputSanitizesAndBlocksNewline(t *testing.T) {
	input := NewTextInput(40, TextInputConfig{Height: 1, SingleLine: true})
	input.Focus()
	input.SetValue("hello\nworld")
	if got := input.Value(); got != "hello world" {
		t.Fatalf("expected single-line SetValue to sanitize newlines, got %q", got)
	}

	controller := textInputModeController{
		input: input,
		keyString: func(tea.KeyMsg) string {
			return "shift+enter"
		},
	}
	handled, _ := controller.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled {
		t.Fatalf("expected shift+enter to be handled for single-line input")
	}
	if got := input.Value(); got != "hello world" {
		t.Fatalf("expected single-line input to ignore newline insertion, got %q", got)
	}
}

func TestDefaultTextInputConfigAutoGrowsToMaxAndShrinks(t *testing.T) {
	input := NewTextInput(40, DefaultTextInputConfig())
	if got := input.Height(); got != 3 {
		t.Fatalf("expected default min height 3, got %d", got)
	}

	input.SetValue("1\n2\n3\n4\n5\n6\n7\n8\n9")
	if got := input.Height(); got != 8 {
		t.Fatalf("expected auto-grow to clamp at max height 8, got %d", got)
	}

	input.SetValue("short")
	if got := input.Height(); got != 3 {
		t.Fatalf("expected auto-grow to shrink back to min height 3, got %d", got)
	}
}

func TestTextInputConfigAutoGrowRespectsCustomBounds(t *testing.T) {
	input := NewTextInput(40, TextInputConfig{
		Height:    2,
		MinHeight: 2,
		MaxHeight: 5,
		AutoGrow:  true,
	})
	if got := input.Height(); got != 2 {
		t.Fatalf("expected custom min height 2, got %d", got)
	}

	input.SetValue("1\n2\n3\n4\n5\n6")
	if got := input.Height(); got != 5 {
		t.Fatalf("expected custom max height 5, got %d", got)
	}
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
