package app

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

const (
	keyScopeNormal                   = "normal"
	keyScopeComposeInput             = "compose_input"
	keyScopeAddNoteInput             = "add_note_input"
	keyScopeSearchInput              = "search_input"
	keyScopeApprovalResponseInput    = "approval_response_input"
	keyScopeRenameInput              = "rename_input"
	keyScopeWorkspaceGroupInput      = "workspace_group_input"
	keyScopeAddWorkspaceInput        = "add_workspace_input"
	keyScopeAddWorktreeInput         = "add_worktree_input"
	keyScopeRecentsReplyInput        = "recents_reply_input"
	keyScopeGuidedWorkflowSetupInput = "guided_workflow_setup_input"
	keyScopeNotesMode                = "notes_mode"
	keyScopePendingApproval          = "pending_approval"
	keyScopeMessageSelect            = "message_selection"
)

type KeybindingConflict struct {
	Key      string
	Scope    string
	Commands []string
}

func (c KeybindingConflict) ToastMessage() string {
	return fmt.Sprintf(
		"keybinding conflict: %s in %s (%s)",
		c.Key,
		c.Scope,
		strings.Join(c.Commands, ", "),
	)
}

func DetectKeybindingConflicts(bindings *Keybindings) []KeybindingConflict {
	if bindings == nil {
		bindings = DefaultKeybindings()
	}
	type scopeKey struct {
		scope string
		key   string
	}
	commandsByScopeKey := map[scopeKey][]string{}
	for _, command := range KnownKeybindingCommands() {
		defaultKey := defaultKeybindingByCommand[command]
		bound := strings.TrimSpace(bindings.KeyFor(command, defaultKey))
		if bound == "" {
			continue
		}
		for _, scope := range keybindingScopesFor(command, bound, defaultKey) {
			k := scopeKey{scope: scope, key: bound}
			commandsByScopeKey[k] = append(commandsByScopeKey[k], command)
		}
	}
	conflicts := make([]KeybindingConflict, 0)
	for scoped, commands := range commandsByScopeKey {
		if len(commands) < 2 {
			continue
		}
		slices.Sort(commands)
		conflicts = append(conflicts, KeybindingConflict{
			Key:      scoped.key,
			Scope:    scoped.scope,
			Commands: commands,
		})
	}
	sort.Slice(conflicts, func(i, j int) bool {
		if conflicts[i].Scope != conflicts[j].Scope {
			return conflicts[i].Scope < conflicts[j].Scope
		}
		if conflicts[i].Key != conflicts[j].Key {
			return conflicts[i].Key < conflicts[j].Key
		}
		return strings.Join(conflicts[i].Commands, ",") < strings.Join(conflicts[j].Commands, ",")
	})
	return conflicts
}

func (m *Model) enqueueStartupKeybindingConflictToasts(conflicts []KeybindingConflict) {
	if len(conflicts) == 0 {
		return
	}
	for _, conflict := range conflicts {
		m.enqueueStartupToast(toastLevelError, conflict.ToastMessage())
	}
}

func keybindingScopesFor(command, boundKey, defaultKey string) []string {
	switch command {
	case KeyCommandQuit:
		return []string{keyScopeNormal, keyScopeNotesMode, keyScopeAddNoteInput, keyScopeMessageSelect}
	case KeyCommandMenu:
		return []string{keyScopeNormal, keyScopeGuidedWorkflowSetupInput}
	case KeyCommandToggleSidebar:
		return []string{keyScopeNormal, keyScopeGuidedWorkflowSetupInput}
	case KeyCommandToggleNotesPanel:
		return []string{keyScopeNormal, keyScopeComposeInput, keyScopeNotesMode, keyScopeAddNoteInput, keyScopeGuidedWorkflowSetupInput}
	case KeyCommandCopySessionID:
		return []string{keyScopeNormal, keyScopeComposeInput}
	case KeyCommandToggleMessageSelect:
		return []string{keyScopeNormal, keyScopeComposeInput, keyScopeMessageSelect}
	case KeyCommandApprove, KeyCommandDecline:
		return []string{keyScopePendingApproval}
	case KeyCommandNotesNew:
		scopes := []string{keyScopeNotesMode}
		if strings.TrimSpace(boundKey) != strings.TrimSpace(defaultKey) {
			scopes = append(scopes, keyScopeNormal, keyScopeComposeInput)
		}
		return scopes
	case KeyCommandComposeModel, KeyCommandComposeReasoning, KeyCommandComposeAccess:
		return []string{keyScopeComposeInput}
	case KeyCommandInputSubmit, KeyCommandInputNewline, KeyCommandInputWordLeft, KeyCommandInputWordRight,
		KeyCommandInputDeleteWordLeft, KeyCommandInputDeleteWordRight, KeyCommandInputSelectAll,
		KeyCommandInputClear,
		KeyCommandInputUndo, KeyCommandInputRedo:
		return []string{
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
	case KeyCommandToggleNotesWorkspace, KeyCommandToggleNotesWorktree, KeyCommandToggleNotesSession, KeyCommandToggleNotesAll:
		return []string{keyScopeNormal, keyScopeNotesMode}
	default:
		return []string{keyScopeNormal}
	}
}
