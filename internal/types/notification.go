package types

import "strings"

type NotificationTrigger string

const (
	NotificationTriggerTurnCompleted NotificationTrigger = "turn.completed"
	NotificationTriggerSessionExited NotificationTrigger = "session.exited"
	NotificationTriggerSessionFailed NotificationTrigger = "session.failed"
	NotificationTriggerSessionKilled NotificationTrigger = "session.killed"
)

type NotificationMethod string

const (
	NotificationMethodAuto       NotificationMethod = "auto"
	NotificationMethodNotifySend NotificationMethod = "notify-send"
	NotificationMethodDunstify   NotificationMethod = "dunstify"
	NotificationMethodBell       NotificationMethod = "bell"
)

type NotificationSettings struct {
	Enabled              bool                  `json:"enabled"`
	Triggers             []NotificationTrigger `json:"triggers,omitempty"`
	Methods              []NotificationMethod  `json:"methods,omitempty"`
	ScriptCommands       []string              `json:"script_commands,omitempty"`
	ScriptTimeoutSeconds int                   `json:"script_timeout_seconds,omitempty"`
	DedupeWindowSeconds  int                   `json:"dedupe_window_seconds,omitempty"`
}

type NotificationSettingsPatch struct {
	Enabled              *bool                 `json:"enabled,omitempty"`
	Triggers             []NotificationTrigger `json:"triggers,omitempty"`
	Methods              []NotificationMethod  `json:"methods,omitempty"`
	ScriptCommands       []string              `json:"script_commands,omitempty"`
	ScriptTimeoutSeconds *int                  `json:"script_timeout_seconds,omitempty"`
	DedupeWindowSeconds  *int                  `json:"dedupe_window_seconds,omitempty"`
}

type NotificationEvent struct {
	Trigger     NotificationTrigger `json:"trigger"`
	OccurredAt  string              `json:"occurred_at"`
	SessionID   string              `json:"session_id,omitempty"`
	WorkspaceID string              `json:"workspace_id,omitempty"`
	WorktreeID  string              `json:"worktree_id,omitempty"`
	Provider    string              `json:"provider,omitempty"`
	Title       string              `json:"title,omitempty"`
	Status      string              `json:"status,omitempty"`
	TurnID      string              `json:"turn_id,omitempty"`
	Cwd         string              `json:"cwd,omitempty"`
	Source      string              `json:"source,omitempty"`
}

func DefaultNotificationSettings() NotificationSettings {
	return NotificationSettings{
		Enabled: true,
		Triggers: []NotificationTrigger{
			NotificationTriggerTurnCompleted,
			NotificationTriggerSessionFailed,
			NotificationTriggerSessionKilled,
			NotificationTriggerSessionExited,
		},
		Methods:              []NotificationMethod{NotificationMethodAuto},
		ScriptTimeoutSeconds: 10,
		DedupeWindowSeconds:  5,
	}
}

func CloneNotificationSettings(in NotificationSettings) NotificationSettings {
	out := in
	if in.Triggers != nil {
		out.Triggers = append([]NotificationTrigger{}, in.Triggers...)
	}
	if in.Methods != nil {
		out.Methods = append([]NotificationMethod{}, in.Methods...)
	}
	if in.ScriptCommands != nil {
		out.ScriptCommands = append([]string{}, in.ScriptCommands...)
	}
	return out
}

func CloneNotificationSettingsPatch(in *NotificationSettingsPatch) *NotificationSettingsPatch {
	if in == nil {
		return nil
	}
	out := *in
	if in.Enabled != nil {
		v := *in.Enabled
		out.Enabled = &v
	}
	if in.Triggers != nil {
		out.Triggers = append([]NotificationTrigger{}, in.Triggers...)
	}
	if in.Methods != nil {
		out.Methods = append([]NotificationMethod{}, in.Methods...)
	}
	if in.ScriptCommands != nil {
		out.ScriptCommands = append([]string{}, in.ScriptCommands...)
	}
	if in.ScriptTimeoutSeconds != nil {
		v := *in.ScriptTimeoutSeconds
		out.ScriptTimeoutSeconds = &v
	}
	if in.DedupeWindowSeconds != nil {
		v := *in.DedupeWindowSeconds
		out.DedupeWindowSeconds = &v
	}
	return &out
}

func MergeNotificationSettings(base NotificationSettings, patch *NotificationSettingsPatch) NotificationSettings {
	out := CloneNotificationSettings(base)
	if patch == nil {
		return NormalizeNotificationSettings(out)
	}
	if patch.Enabled != nil {
		out.Enabled = *patch.Enabled
	}
	if patch.Triggers != nil {
		out.Triggers = append([]NotificationTrigger{}, patch.Triggers...)
	}
	if patch.Methods != nil {
		out.Methods = append([]NotificationMethod{}, patch.Methods...)
	}
	if patch.ScriptCommands != nil {
		out.ScriptCommands = append([]string{}, patch.ScriptCommands...)
	}
	if patch.ScriptTimeoutSeconds != nil {
		out.ScriptTimeoutSeconds = *patch.ScriptTimeoutSeconds
	}
	if patch.DedupeWindowSeconds != nil {
		out.DedupeWindowSeconds = *patch.DedupeWindowSeconds
	}
	return NormalizeNotificationSettings(out)
}

func NormalizeNotificationSettings(in NotificationSettings) NotificationSettings {
	out := in
	out.Triggers = normalizeNotificationTriggers(in.Triggers)
	if len(out.Triggers) == 0 {
		out.Triggers = append([]NotificationTrigger{}, DefaultNotificationSettings().Triggers...)
	}
	out.Methods = normalizeNotificationMethods(in.Methods)
	if len(out.Methods) == 0 {
		out.Methods = append([]NotificationMethod{}, DefaultNotificationSettings().Methods...)
	}
	out.ScriptCommands = normalizeStringList(in.ScriptCommands)
	if out.ScriptTimeoutSeconds <= 0 {
		out.ScriptTimeoutSeconds = DefaultNotificationSettings().ScriptTimeoutSeconds
	}
	if out.DedupeWindowSeconds <= 0 {
		out.DedupeWindowSeconds = DefaultNotificationSettings().DedupeWindowSeconds
	}
	return out
}

func normalizeNotificationTriggers(values []NotificationTrigger) []NotificationTrigger {
	if len(values) == 0 {
		return nil
	}
	seen := map[NotificationTrigger]struct{}{}
	out := make([]NotificationTrigger, 0, len(values))
	for _, value := range values {
		normalized, ok := NormalizeNotificationTrigger(string(value))
		if !ok {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func normalizeNotificationMethods(values []NotificationMethod) []NotificationMethod {
	if len(values) == 0 {
		return nil
	}
	seen := map[NotificationMethod]struct{}{}
	out := make([]NotificationMethod, 0, len(values))
	for _, value := range values {
		normalized, ok := NormalizeNotificationMethod(string(value))
		if !ok {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func normalizeStringList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func NormalizeNotificationTrigger(raw string) (NotificationTrigger, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "turn.completed", "turn_completed", "turn-completed":
		return NotificationTriggerTurnCompleted, true
	case "session.exited", "session_exited", "session-exited":
		return NotificationTriggerSessionExited, true
	case "session.failed", "session_failed", "session-failed":
		return NotificationTriggerSessionFailed, true
	case "session.killed", "session_killed", "session-killed":
		return NotificationTriggerSessionKilled, true
	default:
		return "", false
	}
}

func NormalizeNotificationMethod(raw string) (NotificationMethod, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "auto":
		return NotificationMethodAuto, true
	case "notify-send", "notify_send", "notifysend":
		return NotificationMethodNotifySend, true
	case "dunstify":
		return NotificationMethodDunstify, true
	case "bell", "terminal-bell", "terminal_bell":
		return NotificationMethodBell, true
	default:
		return "", false
	}
}

func NotificationTriggerEnabled(settings NotificationSettings, trigger NotificationTrigger) bool {
	for _, candidate := range settings.Triggers {
		if candidate == trigger {
			return true
		}
	}
	return false
}
