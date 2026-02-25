package app

import (
	"testing"
	"time"

	"control/internal/types"
)

func TestRegisterSidebarSortKeyAddsExtensibleSortKey(t *testing.T) {
	const customKey sidebarSortKey = "priority"
	RegisterSidebarSortKey(SidebarSortKeySpec{
		Key:   customKey,
		Label: "Priority",
		Less: func(_ sidebarSortWorkspaceContext, left, right *types.Workspace) bool {
			if left == nil || right == nil {
				return left != nil
			}
			return left.ID > right.ID
		},
	})
	if got := parseSidebarSortKey(string(customKey)); got != customKey {
		t.Fatalf("expected registered key %q, got %q", customKey, got)
	}
	if got := sidebarSortLabel(customKey); got != "Priority" {
		t.Fatalf("expected registered label Priority, got %q", got)
	}

	a := &types.Workspace{ID: "a", CreatedAt: time.Now().UTC()}
	b := &types.Workspace{ID: "b", CreatedAt: time.Now().UTC()}
	if !sidebarSortLess(customKey, sidebarSortWorkspaceContext{}, b, a) {
		t.Fatalf("expected custom less comparator to be used")
	}
}

func TestDefaultSidebarSortPolicyNormalizeFallsBackToCreated(t *testing.T) {
	policy := defaultSidebarSortPolicy{}
	state := policy.Normalize(sidebarSortState{Key: sidebarSortKey("unknown")})
	if got := state.Key; got != sidebarSortKeyCreated {
		t.Fatalf("expected created fallback, got %q", got)
	}
}

func TestDefaultSidebarSortPolicyToggleAndLabelAndCycle(t *testing.T) {
	policy := defaultSidebarSortPolicy{}
	toggled := policy.ToggleReverse(sidebarSortState{Key: sidebarSortKeyCreated})
	if !toggled.Reverse {
		t.Fatalf("expected reverse toggle on")
	}
	cycled := policy.Cycle(sidebarSortState{Key: sidebarSortKeyCreated}, 1)
	if cycled.Key == sidebarSortKeyCreated {
		t.Fatalf("expected cycle to move key")
	}
	if got := policy.Label(sidebarSortKeyActivity); got != "Activity" {
		t.Fatalf("expected activity label, got %q", got)
	}
}

func TestSortedSidebarSortKeysContainsDefaultsAndCustom(t *testing.T) {
	RegisterSidebarSortKey(SidebarSortKeySpec{Key: "aaa_custom", Label: "AAA"})
	keys := sortedSidebarSortKeys()
	if len(keys) < 4 {
		t.Fatalf("expected at least four keys including custom, got %d", len(keys))
	}
	foundDefault := false
	foundCustom := false
	for _, key := range keys {
		if key == sidebarSortKeyCreated {
			foundDefault = true
		}
		if key == sidebarSortKey("aaa_custom") {
			foundCustom = true
		}
	}
	if !foundDefault || !foundCustom {
		t.Fatalf("expected defaults and custom keys in sorted list, got %#v", keys)
	}
}

func TestFormatKeyBadgeTransformsModifiersAndTokens(t *testing.T) {
	if got := formatKeyBadge("ctrl+f"); got != "[Ctrl+F]" {
		t.Fatalf("expected ctrl+f badge, got %q", got)
	}
	if got := formatKeyBadge("alt+left"); got != "[Alt+Left]" {
		t.Fatalf("expected alt+left badge, got %q", got)
	}
	if got := formatKeyBadge(""); got != "" {
		t.Fatalf("expected empty badge for empty key, got %q", got)
	}
}
