package types

import "testing"

func TestMergeNotificationSettingsAppliesPatch(t *testing.T) {
	base := DefaultNotificationSettings()
	enabled := false
	scriptTimeout := 3
	window := 2
	got := MergeNotificationSettings(base, &NotificationSettingsPatch{
		Enabled:              &enabled,
		Triggers:             []NotificationTrigger{NotificationTriggerSessionFailed},
		Methods:              []NotificationMethod{NotificationMethodBell},
		ScriptCommands:       []string{"echo done"},
		ScriptTimeoutSeconds: &scriptTimeout,
		DedupeWindowSeconds:  &window,
	})
	if got.Enabled {
		t.Fatalf("expected disabled override")
	}
	if len(got.Triggers) != 1 || got.Triggers[0] != NotificationTriggerSessionFailed {
		t.Fatalf("unexpected triggers: %#v", got.Triggers)
	}
	if len(got.Methods) != 1 || got.Methods[0] != NotificationMethodBell {
		t.Fatalf("unexpected methods: %#v", got.Methods)
	}
	if len(got.ScriptCommands) != 1 || got.ScriptCommands[0] != "echo done" {
		t.Fatalf("unexpected scripts: %#v", got.ScriptCommands)
	}
	if got.ScriptTimeoutSeconds != 3 {
		t.Fatalf("unexpected script timeout: %d", got.ScriptTimeoutSeconds)
	}
	if got.DedupeWindowSeconds != 2 {
		t.Fatalf("unexpected dedupe window: %d", got.DedupeWindowSeconds)
	}
}

func TestNormalizeNotificationSettingsFallsBackToDefaults(t *testing.T) {
	got := NormalizeNotificationSettings(NotificationSettings{
		Enabled:  true,
		Triggers: []NotificationTrigger{"bad"},
		Methods:  []NotificationMethod{"unknown"},
	})
	if len(got.Triggers) == 0 {
		t.Fatalf("expected fallback triggers")
	}
	if len(got.Methods) == 0 || got.Methods[0] != NotificationMethodAuto {
		t.Fatalf("expected fallback method auto, got %#v", got.Methods)
	}
	if got.ScriptTimeoutSeconds <= 0 {
		t.Fatalf("expected default script timeout")
	}
	if got.DedupeWindowSeconds <= 0 {
		t.Fatalf("expected default dedupe window")
	}
}
