package guidedworkflows

import "testing"

func TestNormalizePolicyPreset(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want PolicyPreset
		ok   bool
	}{
		{name: "empty", in: "", want: "", ok: true},
		{name: "low", in: "LOW", want: PolicyPresetLow, ok: true},
		{name: "balanced", in: "balanced", want: PolicyPresetBalanced, ok: true},
		{name: "medium_alias", in: " medium ", want: PolicyPresetBalanced, ok: true},
		{name: "default_alias", in: "default", want: PolicyPresetBalanced, ok: true},
		{name: "high", in: "high", want: PolicyPresetHigh, ok: true},
		{name: "invalid", in: "weird", want: "", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := NormalizePolicyPreset(tt.in)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("NormalizePolicyPreset(%q) = (%q, %v), want (%q, %v)", tt.in, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestPolicyOverrideForPreset(t *testing.T) {
	low := PolicyOverrideForPreset(PolicyPresetLow)
	if low == nil || low.ConfidenceThreshold == nil || *low.ConfidenceThreshold != PolicyPresetLowConfidenceThreshold {
		t.Fatalf("unexpected low preset confidence override: %#v", low)
	}
	if low.PauseThreshold == nil || *low.PauseThreshold != PolicyPresetLowPauseThreshold {
		t.Fatalf("unexpected low preset pause override: %#v", low)
	}

	high := PolicyOverrideForPreset(PolicyPresetHigh)
	if high == nil || high.ConfidenceThreshold == nil || *high.ConfidenceThreshold != PolicyPresetHighConfidenceThreshold {
		t.Fatalf("unexpected high preset confidence override: %#v", high)
	}
	if high.PauseThreshold == nil || *high.PauseThreshold != PolicyPresetHighPauseThreshold {
		t.Fatalf("unexpected high preset pause override: %#v", high)
	}

	if got := PolicyOverrideForPreset(PolicyPresetBalanced); got != nil {
		t.Fatalf("expected balanced preset to keep default thresholds, got %#v", got)
	}
}
