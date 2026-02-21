package app

import (
	"strings"
	"testing"
)

func TestDetectKeybindingConflictsSkipsCrossScopeOverlap(t *testing.T) {
	bindings := NewKeybindings(map[string]string{
		KeyCommandKillSession: "y",
	})

	conflicts := DetectKeybindingConflicts(bindings)
	for _, conflict := range conflicts {
		if conflict.Key == "y" {
			t.Fatalf("expected no conflict for y across normal and pending approval scopes: %#v", conflict)
		}
	}
}

func TestDetectKeybindingConflictsDefaultsAreClean(t *testing.T) {
	conflicts := DetectKeybindingConflicts(DefaultKeybindings())
	if len(conflicts) != 0 {
		t.Fatalf("expected no default conflicts, got %#v", conflicts)
	}
}

func TestDetectKeybindingConflictsDetectsNormalScopeConflict(t *testing.T) {
	bindings := NewKeybindings(map[string]string{
		KeyCommandNewSession: "ctrl+n",
		KeyCommandNotesNew:   "ctrl+n",
	})

	conflicts := DetectKeybindingConflicts(bindings)
	if len(conflicts) == 0 {
		t.Fatalf("expected conflict for ctrl+n in normal scope")
	}
	found := false
	for _, conflict := range conflicts {
		if conflict.Scope == keyScopeNormal && conflict.Key == "ctrl+n" {
			if len(conflict.Commands) != 2 || conflict.Commands[0] != KeyCommandNewSession || conflict.Commands[1] != KeyCommandNotesNew {
				t.Fatalf("unexpected commands in conflict: %#v", conflict)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("expected normal scope conflict for ctrl+n, got %#v", conflicts)
	}
}

func TestDetectKeybindingConflictsDetectsComposeInputConflict(t *testing.T) {
	bindings := NewKeybindings(map[string]string{
		KeyCommandInputSubmit: "ctrl+1",
	})

	conflicts := DetectKeybindingConflicts(bindings)
	found := false
	for _, conflict := range conflicts {
		if conflict.Scope == keyScopeComposeInput && conflict.Key == "ctrl+1" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected compose input conflict for ctrl+1, got %#v", conflicts)
	}
}

func TestDetectKeybindingConflictsDetectsSearchInputConflictForInputClear(t *testing.T) {
	bindings := NewKeybindings(map[string]string{
		KeyCommandInputClear: "enter",
	})

	conflicts := DetectKeybindingConflicts(bindings)
	found := false
	for _, conflict := range conflicts {
		if conflict.Scope == keyScopeSearchInput && conflict.Key == "enter" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected search input conflict for enter, got %#v", conflicts)
	}
}

func TestDetectKeybindingConflictsLegacyComposeClearAliasNormalizesToInputClear(t *testing.T) {
	bindings := NewKeybindings(map[string]string{
		KeyCommandComposeClearInput: "enter",
	})

	conflicts := DetectKeybindingConflicts(bindings)
	found := false
	for _, conflict := range conflicts {
		if conflict.Scope == keyScopeSearchInput && conflict.Key == "enter" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected legacy compose clear alias to conflict as input clear, got %#v", conflicts)
	}
}

func TestDetectKeybindingConflictsInputClearScopesCoverAllInputContexts(t *testing.T) {
	bindings := NewKeybindings(map[string]string{
		KeyCommandInputClear: "enter",
	})

	conflicts := DetectKeybindingConflicts(bindings)
	expectedScopes := []string{
		keyScopeComposeInput,
		keyScopeAddNoteInput,
		keyScopeSearchInput,
		keyScopeApprovalResponseInput,
		keyScopeRenameInput,
		keyScopeWorkspaceGroupInput,
		keyScopeAddWorkspaceInput,
		keyScopeAddWorktreeInput,
		keyScopeRecentsReplyInput,
		keyScopeGuidedWorkflowSetupInput,
	}
	for _, scope := range expectedScopes {
		if !hasConflictWithCommand(conflicts, scope, "enter", KeyCommandInputClear) {
			t.Fatalf("expected input clear conflict in scope %q, got %#v", scope, conflicts)
		}
	}
}

func hasConflictWithCommand(conflicts []KeybindingConflict, scope, key, command string) bool {
	for _, conflict := range conflicts {
		if conflict.Scope != scope || conflict.Key != key {
			continue
		}
		for _, candidate := range conflict.Commands {
			if candidate == command {
				return true
			}
		}
	}
	return false
}

func TestKeybindingConflictToastMessageIncludesKeyAndCommands(t *testing.T) {
	conflict := KeybindingConflict{
		Key:      "ctrl+n",
		Scope:    keyScopeNormal,
		Commands: []string{KeyCommandNewSession, KeyCommandNotesNew},
	}

	text := conflict.ToastMessage()
	if text == "" {
		t.Fatalf("expected non-empty toast message")
	}
	if want := "ctrl+n"; !strings.Contains(text, want) {
		t.Fatalf("expected toast message to contain key %q: %q", want, text)
	}
	if want := KeyCommandNotesNew; !strings.Contains(text, want) {
		t.Fatalf("expected toast message to contain command %q: %q", want, text)
	}
}

func TestEnqueueStartupKeybindingConflictToastsShowsFirstImmediately(t *testing.T) {
	m := NewModel(nil)
	conflicts := []KeybindingConflict{
		{Key: "ctrl+n", Scope: keyScopeNormal, Commands: []string{KeyCommandNewSession, KeyCommandNotesNew}},
		{Key: "ctrl+1", Scope: keyScopeComposeInput, Commands: []string{KeyCommandComposeModel, KeyCommandInputSubmit}},
	}

	m.enqueueStartupKeybindingConflictToasts(conflicts)
	if m.toastText == "" {
		t.Fatalf("expected first conflict toast to be shown")
	}
	if !strings.Contains(m.toastText, "ctrl+n") {
		t.Fatalf("expected first conflict key in toast, got %q", m.toastText)
	}
	if len(m.startupToasts) != 1 {
		t.Fatalf("expected one remaining startup toast, got %d", len(m.startupToasts))
	}
}
