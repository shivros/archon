package app

import (
	"testing"
)

func TestResolveSidebarWidthUsesPreference(t *testing.T) {
	pref := &SplitPreference{Columns: 30}
	if got := resolveSidebarWidth(120, false, pref); got != 30 {
		t.Fatalf("expected preferred sidebar width 30, got %d", got)
	}
}

func TestResolveSidePanelWidthUsesExplicitPanelWidthBeforeSplit(t *testing.T) {
	split := &SplitPreference{Columns: 42}
	if got := resolveSidePanelWidth(140, split, 36); got != 36 {
		t.Fatalf("expected explicit panel width 36, got %d", got)
	}
}

func TestCaptureSplitPreferenceStoresColumnsAndRatio(t *testing.T) {
	pref := captureSplitPreference(160, 40, nil)
	if pref == nil {
		t.Fatalf("expected split preference")
	}
	if pref.Columns != 40 {
		t.Fatalf("expected columns 40, got %d", pref.Columns)
	}
	if pref.Ratio <= 0 || pref.Ratio >= 1 {
		t.Fatalf("expected ratio within (0,1), got %f", pref.Ratio)
	}
}

func TestSanitizeSplitPreferenceNilAndInvalidValues(t *testing.T) {
	if got := sanitizeSplitPreference(nil); got != nil {
		t.Fatalf("expected nil preference to remain nil")
	}

	pref := &SplitPreference{Columns: -4, Ratio: 2.0}
	if got := sanitizeSplitPreference(pref); got != nil {
		t.Fatalf("expected invalid preference to sanitize to nil, got %#v", got)
	}
}

func TestSanitizeSplitPreferencePreservesValidRatioWhenColumnsInvalid(t *testing.T) {
	pref := &SplitPreference{Columns: -2, Ratio: 0.3}
	got := sanitizeSplitPreference(pref)
	if got == nil {
		t.Fatalf("expected non-nil sanitized preference")
	}
	if got.Columns != 0 {
		t.Fatalf("expected columns to clamp to 0, got %d", got.Columns)
	}
	if got.Ratio != 0.3 {
		t.Fatalf("expected ratio preserved, got %f", got.Ratio)
	}
}

func TestCloneSplitPreferenceCopiesValues(t *testing.T) {
	original := &SplitPreference{Columns: 28, Ratio: 0.4}
	clone := cloneSplitPreference(original)
	if clone == nil {
		t.Fatalf("expected clone")
	}
	if clone == original {
		t.Fatalf("expected clone to allocate new instance")
	}
	if clone.Columns != original.Columns || clone.Ratio != original.Ratio {
		t.Fatalf("expected clone values to match original")
	}
}

func TestPreferredSplitColumnsUsesRatioAndFallback(t *testing.T) {
	ratioPref := &SplitPreference{Ratio: 0.25}
	if got := preferredSplitColumns(200, 20, 80, 30, ratioPref); got != 50 {
		t.Fatalf("expected ratio-based width 50, got %d", got)
	}

	invalidPref := &SplitPreference{Ratio: 2.0}
	if got := preferredSplitColumns(200, 20, 80, 30, invalidPref); got != 30 {
		t.Fatalf("expected fallback width 30 for invalid ratio, got %d", got)
	}
}

func TestClampSidebarAndPanelWidthGuards(t *testing.T) {
	if got := clampSidebarWidthForTerminal(0, 20); got != 0 {
		t.Fatalf("expected sidebar width 0 for zero terminal width, got %d", got)
	}
	if got := clampSidePanelWidthForViewport(0, 20); got != 0 {
		t.Fatalf("expected panel width 0 for zero viewport width, got %d", got)
	}
}
