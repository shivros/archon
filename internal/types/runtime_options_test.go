package types

import "testing"

func TestNormalizeReasoningLevel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input ReasoningLevel
		want  ReasoningLevel
		ok    bool
	}{
		{
			name:  "low",
			input: "low",
			want:  ReasoningLow,
			ok:    true,
		},
		{
			name:  "trimmed uppercase extra high",
			input: "  EXTRA_HIGH  ",
			want:  ReasoningExtraHigh,
			ok:    true,
		},
		{
			name:  "hyphenated high",
			input: "HIGH",
			want:  ReasoningHigh,
			ok:    true,
		},
		{
			name:  "hyphen separator converts",
			input: "extra-high",
			want:  ReasoningExtraHigh,
			ok:    true,
		},
		{
			name:  "invalid value",
			input: "turbo",
			want:  "",
			ok:    false,
		},
		{
			name:  "empty",
			input: "",
			want:  "",
			ok:    false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := NormalizeReasoningLevel(tc.input)
			if ok != tc.ok {
				t.Fatalf("expected ok=%t, got %t", tc.ok, ok)
			}
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestNormalizeAccessLevel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input AccessLevel
		want  AccessLevel
		ok    bool
	}{
		{
			name:  "empty allowed",
			input: "",
			want:  "",
			ok:    true,
		},
		{
			name:  "read only canonical",
			input: "read_only",
			want:  AccessReadOnly,
			ok:    true,
		},
		{
			name:  "read only alias",
			input: "readonly",
			want:  AccessReadOnly,
			ok:    true,
		},
		{
			name:  "on request alias",
			input: "onrequest",
			want:  AccessOnRequest,
			ok:    true,
		},
		{
			name:  "on request with hyphen",
			input: "ON-REQUEST",
			want:  AccessOnRequest,
			ok:    true,
		},
		{
			name:  "full access alias",
			input: "fullaccess",
			want:  AccessFull,
			ok:    true,
		},
		{
			name:  "full access canonical",
			input: " full_access ",
			want:  AccessFull,
			ok:    true,
		},
		{
			name:  "invalid value",
			input: "sandboxed",
			want:  "",
			ok:    false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := NormalizeAccessLevel(tc.input)
			if ok != tc.ok {
				t.Fatalf("expected ok=%t, got %t", tc.ok, ok)
			}
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}
