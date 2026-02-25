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

func TestTextInputModeControllerShiftEnterModifierWithoutLiteralKeyStringInsertsNewline(t *testing.T) {
	input := NewTextInput(40, TextInputConfig{Height: 3})
	input.Focus()
	input.SetValue("hello")

	controller := textInputModeController{
		input: input,
		keyString: func(tea.KeyMsg) string {
			// Some terminals report Shift+Enter as "enter" while still setting the Shift modifier.
			return "enter"
		},
	}

	handled, _ := controller.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift})
	if !handled {
		t.Fatalf("expected shift+enter modifier fallback to be handled")
	}
	if got := input.Value(); got != "hello\n" {
		t.Fatalf("expected newline insert from shift+enter modifier fallback, got %q", got)
	}
}

func TestTextInputModeControllerCtrlJInsertsNewline(t *testing.T) {
	input := NewTextInput(40, TextInputConfig{Height: 3})
	input.Focus()
	input.SetValue("hello")

	controller := textInputModeController{
		input: input,
		keyString: func(msg tea.KeyMsg) string {
			return msg.String()
		},
	}

	handled, _ := controller.Update(tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl})
	if !handled {
		t.Fatalf("expected ctrl+j to be handled")
	}
	if got := input.Value(); got != "hello\n" {
		t.Fatalf("expected newline insert, got %q", got)
	}
}

func TestTextInputModeControllerLineUpAndDownMoveCursorLine(t *testing.T) {
	upInput := NewTextInput(40, TextInputConfig{Height: 3})
	upInput.Focus()
	upInput.SetValue("line1\nline2")
	upController := textInputModeController{
		input: upInput,
		keyString: func(msg tea.KeyMsg) string {
			return msg.String()
		},
	}

	handled, _ := upController.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if !handled {
		t.Fatalf("expected up to be handled")
	}
	_, _ = upController.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if got := upInput.Value(); got != "line1x\nline2" {
		t.Fatalf("expected up to move cursor to prior line, got %q", got)
	}

	downInput := NewTextInput(40, TextInputConfig{Height: 3})
	downInput.Focus()
	downInput.SetValue("line1\nline2")
	_ = downInput.MoveLineUp()
	downController := textInputModeController{
		input: downInput,
		keyString: func(msg tea.KeyMsg) string {
			return msg.String()
		},
	}

	handled, _ = downController.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if !handled {
		t.Fatalf("expected down to be handled")
	}
	_, _ = downController.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if got := downInput.Value(); got != "line1\nline2x" {
		t.Fatalf("expected down to move cursor to next line, got %q", got)
	}
}

func TestTextInputModeControllerSupportsRemappedLineUpCommand(t *testing.T) {
	input := NewTextInput(40, TextInputConfig{Height: 3})
	input.Focus()
	input.SetValue("line1\nline2")

	controller := textInputModeController{
		input: input,
		keyString: func(msg tea.KeyMsg) string {
			return msg.String()
		},
		keyMatchesCommand: func(msg tea.KeyMsg, command, fallback string) bool {
			return command == KeyCommandInputLineUp && msg.String() == "f7"
		},
	}

	handled, _ := controller.Update(tea.KeyPressMsg{Code: tea.KeyF7})
	if !handled {
		t.Fatalf("expected remapped line-up command to be handled")
	}
	_, _ = controller.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if got := input.Value(); got != "line1x\nline2" {
		t.Fatalf("expected remapped line-up to move cursor, got %q", got)
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

func TestTextInputModeControllerCtrlCClearsInput(t *testing.T) {
	input := NewTextInput(40, TextInputConfig{Height: 3})
	input.SetValue("clear me")
	cleared := false

	controller := textInputModeController{
		input: input,
		keyString: func(msg tea.KeyMsg) string {
			return msg.String()
		},
		onClear: func() tea.Cmd {
			cleared = true
			return nil
		},
	}

	handled, cmd := controller.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if !handled {
		t.Fatalf("expected ctrl+c to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no command for clear action")
	}
	if !cleared {
		t.Fatalf("expected clear callback to be invoked")
	}
	if got := input.Value(); got != "" {
		t.Fatalf("expected input to be cleared, got %q", got)
	}
}

func TestTextInputModeControllerSupportsRemappedClearCommand(t *testing.T) {
	input := NewTextInput(40, TextInputConfig{Height: 3})
	input.SetValue("clear me")
	cleared := false

	controller := textInputModeController{
		input: input,
		keyString: func(msg tea.KeyMsg) string {
			return msg.String()
		},
		keyMatchesCommand: func(msg tea.KeyMsg, command, fallback string) bool {
			return command == KeyCommandInputClear && msg.String() == "f7"
		},
		onClear: func() tea.Cmd {
			cleared = true
			return nil
		},
	}

	handled, _ := controller.Update(tea.KeyPressMsg{Code: tea.KeyF7})
	if !handled {
		t.Fatalf("expected remapped clear command to be handled")
	}
	if !cleared {
		t.Fatalf("expected clear callback to be invoked")
	}
	if got := input.Value(); got != "" {
		t.Fatalf("expected input to be cleared, got %q", got)
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

	layout := BuildInputPanelLayout(InputPanel{
		Input: input,
		Footer: InputFooterFunc(func() string {
			return "Model: gpt-5"
		}),
	})
	line, _ := layout.View()
	if !strings.Contains(line, "hello") {
		t.Fatalf("expected base input line in view, got %q", line)
	}
	if !strings.Contains(line, "Model: gpt-5") {
		t.Fatalf("expected composed footer in view, got %q", line)
	}
}

func TestInputPanelFrameAppliesOnlyToInputSegment(t *testing.T) {
	input := NewTextInput(40, TextInputConfig{Height: 2})
	input.SetValue("hello")

	panel := InputPanel{
		Input: input,
		Footer: InputFooterFunc(func() string {
			return "Model: gpt-5"
		}),
		Frame: LipglossInputPanelFrame{Style: guidedWorkflowPromptFrameStyle},
	}

	layout := BuildInputPanelLayout(panel)
	line, _ := layout.View()
	lines := strings.Split(line, "\n")
	if len(lines) < 2 {
		t.Fatalf("expected framed input plus footer, got %q", line)
	}
	footerLine := lines[len(lines)-1]
	if !strings.Contains(footerLine, "Model: gpt-5") {
		t.Fatalf("expected footer line to remain visible, got %q", footerLine)
	}
	if strings.Contains(footerLine, "│") || strings.Contains(footerLine, "╰") || strings.Contains(footerLine, "╯") {
		t.Fatalf("expected footer to render outside the frame, got %q", footerLine)
	}
	inputLines := layout.InputLineCount()
	wantInputLines := input.Height() + guidedWorkflowPromptFrameStyle.GetVerticalFrameSize()
	if inputLines != wantInputLines {
		t.Fatalf("expected framed input line count %d, got %d", wantInputLines, inputLines)
	}
	footerRow, ok := layout.FooterStartRow()
	if !ok {
		t.Fatalf("expected footer row for framed panel")
	}
	if footerRow != inputLines {
		t.Fatalf("expected footer row %d, got %d", inputLines, footerRow)
	}
	if got, want := layout.LineCount(), inputLines+1; got != want {
		t.Fatalf("expected total line count %d, got %d", want, got)
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

func TestTextInputModeControllerShouldPassthroughReleasesKey(t *testing.T) {
	input := NewTextInput(40, TextInputConfig{Height: 3})
	input.Focus()
	input.SetValue("hello")

	controller := textInputModeController{
		input: input,
		keyString: func(msg tea.KeyMsg) string {
			return msg.String()
		},
		shouldPassthrough: func(msg tea.KeyMsg) bool {
			return msg.String() == "ctrl+t"
		},
	}

	handled, _ := controller.Update(tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl})
	if handled {
		t.Fatalf("expected ctrl+t to be released by shouldPassthrough")
	}
	if got := input.Value(); got != "hello" {
		t.Fatalf("expected input unchanged, got %q", got)
	}
}

func TestTextInputModeControllerShouldPassthroughNotCalledForInputCommands(t *testing.T) {
	input := NewTextInput(40, TextInputConfig{Height: 3})
	input.Focus()
	input.SetValue("hello world")

	passthroughCalled := false
	controller := textInputModeController{
		input: input,
		keyString: func(msg tea.KeyMsg) string {
			return msg.String()
		},
		shouldPassthrough: func(msg tea.KeyMsg) bool {
			passthroughCalled = true
			return true
		},
	}

	// ctrl+a matches InputSelectAll which is handled before shouldPassthrough
	handled, _ := controller.Update(tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl})
	if !handled {
		t.Fatalf("expected ctrl+a to be handled as select all")
	}
	if passthroughCalled {
		t.Fatalf("expected shouldPassthrough not to be called for recognized input commands")
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
