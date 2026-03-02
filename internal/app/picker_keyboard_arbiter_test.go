package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

type arbiterTestMsg struct{}

func TestPickerKeyboardArbiterConsumesPrintableTextViaTypeAhead(t *testing.T) {
	arbiter := newPickerKeyboardArbiter(nil, nil, defaultPickerPasteNormalizer{})
	picker := NewSelectPicker(40, 5)
	picker.SetOptions([]selectOption{{id: "a", label: "Alpha"}})

	handled, cmd := arbiter.Handle(tea.KeyPressMsg{Code: 'h', Text: "h"}, picker, pickerKeyboardHooks{})
	if !handled {
		t.Fatalf("expected printable key to be handled by arbiter")
	}
	if cmd != nil {
		t.Fatalf("expected no command for typeahead text")
	}
	if got := picker.Query(); got != "h" {
		t.Fatalf("expected query to update, got %q", got)
	}
}

func TestPickerKeyboardArbiterRoutesNavigationHooks(t *testing.T) {
	arbiter := newPickerKeyboardArbiter(nil, nil, defaultPickerPasteNormalizer{})
	downCalls := 0
	upCalls := 0

	handled, _ := arbiter.Handle(tea.KeyPressMsg{Code: tea.KeyDown}, nil, pickerKeyboardHooks{
		MoveDown: func() { downCalls++ },
		MoveUp:   func() { upCalls++ },
	})
	if !handled {
		t.Fatalf("expected down key to be handled")
	}
	if downCalls != 1 || upCalls != 0 {
		t.Fatalf("unexpected move counts: down=%d up=%d", downCalls, upCalls)
	}

	handled, _ = arbiter.Handle(tea.KeyPressMsg{Code: tea.KeyUp}, nil, pickerKeyboardHooks{
		MoveDown: func() { downCalls++ },
		MoveUp:   func() { upCalls++ },
	})
	if !handled {
		t.Fatalf("expected up key to be handled")
	}
	if downCalls != 1 || upCalls != 1 {
		t.Fatalf("unexpected move counts after up: down=%d up=%d", downCalls, upCalls)
	}
}

func TestPickerKeyboardArbiterPreHandleCanOverrideDefaultRouting(t *testing.T) {
	arbiter := newPickerKeyboardArbiter(nil, nil, defaultPickerPasteNormalizer{})
	expected := tea.Cmd(func() tea.Msg { return arbiterTestMsg{} })

	handled, cmd := arbiter.Handle(tea.KeyPressMsg{Code: tea.KeyEnter}, nil, pickerKeyboardHooks{
		PreHandleKey: func(key string, msg tea.KeyMsg) (bool, tea.Cmd) {
			if key == "enter" {
				return true, expected
			}
			return false, nil
		},
		Confirm: func() tea.Cmd {
			t.Fatalf("confirm hook should be bypassed when pre-handle consumes key")
			return nil
		},
	})
	if !handled {
		t.Fatalf("expected pre-handle override to consume key")
	}
	if cmd == nil {
		t.Fatalf("expected pre-handle command to be returned")
	}
	if _, ok := cmd().(arbiterTestMsg); !ok {
		t.Fatalf("expected arbiterTestMsg command result, got %T", cmd())
	}
}

func TestPickerKeyboardArbiterRoutesToggleAndPagingHooks(t *testing.T) {
	arbiter := newPickerKeyboardArbiter(nil, nil, defaultPickerPasteNormalizer{})
	toggles := 0
	pageUps := 0
	pageDowns := 0
	homes := 0
	ends := 0

	cases := []struct {
		msg tea.KeyMsg
	}{
		{tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}},
		{tea.KeyPressMsg{Code: tea.KeyPgUp}},
		{tea.KeyPressMsg{Code: tea.KeyPgDown}},
		{tea.KeyPressMsg{Code: tea.KeyHome}},
		{tea.KeyPressMsg{Code: tea.KeyEnd}},
	}
	for _, tc := range cases {
		handled, _ := arbiter.Handle(tc.msg, nil, pickerKeyboardHooks{
			Toggle:   func() { toggles++ },
			PageUp:   func() { pageUps++ },
			PageDown: func() { pageDowns++ },
			Home:     func() { homes++ },
			End:      func() { ends++ },
		})
		if !handled {
			t.Fatalf("expected %q to be handled", tc.msg.String())
		}
	}
	if toggles != 1 || pageUps != 1 || pageDowns != 1 || homes != 1 || ends != 1 {
		t.Fatalf("unexpected hook counts: toggle=%d pgup=%d pgdown=%d home=%d end=%d", toggles, pageUps, pageDowns, homes, ends)
	}
}

func TestPickerKeyboardArbiterPasteInvokesTypeAheadHook(t *testing.T) {
	arbiter := newPickerKeyboardArbiter(nil, nil, defaultPickerPasteNormalizer{})
	picker := NewSelectPicker(40, 5)
	picker.SetOptions([]selectOption{{id: "a", label: "Alpha"}})
	typeAheadCalls := 0

	handled, cmd := arbiter.Handle(tea.PasteMsg{Content: "alp"}, picker, pickerKeyboardHooks{
		OnTypeAhead: func() { typeAheadCalls++ },
	})
	if !handled {
		t.Fatalf("expected paste to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no command for paste")
	}
	if typeAheadCalls != 1 {
		t.Fatalf("expected OnTypeAhead to be called once, got %d", typeAheadCalls)
	}
	if got := picker.Query(); got != "alp" {
		t.Fatalf("expected query to update from paste, got %q", got)
	}
}

func TestPickerKeyboardArbiterReturnsUnhandledForUnsupportedMessages(t *testing.T) {
	arbiter := newPickerKeyboardArbiter(nil, nil, defaultPickerPasteNormalizer{})

	handled, cmd := arbiter.Handle(struct{}{}, nil, pickerKeyboardHooks{})
	if handled {
		t.Fatalf("expected non-key/paste message to be unhandled")
	}
	if cmd != nil {
		t.Fatalf("expected no command for unhandled message")
	}

	handled, cmd = arbiter.Handle(tea.KeyPressMsg{Code: 'x', Text: "x"}, nil, pickerKeyboardHooks{})
	if handled {
		t.Fatalf("expected key without hooks/picker to be unhandled")
	}
	if cmd != nil {
		t.Fatalf("expected no command for unhandled key")
	}
}
