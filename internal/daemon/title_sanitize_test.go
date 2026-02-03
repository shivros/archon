package daemon

import "testing"

func TestSanitizeTitle(t *testing.T) {
	tests := []struct {
		input  string
		expect string
	}{
		{" hello\nworld\t", "hello world"},
		{"\t", ""},
		{"Title with\u0000null", "Title withnull"},
		{"Emoji 🚀 title", "Emoji title"},
		{"multi   space", "multi space"},
	}
	for _, tc := range tests {
		got := sanitizeTitle(tc.input)
		if got != tc.expect {
			t.Fatalf("sanitizeTitle(%q) = %q, want %q", tc.input, got, tc.expect)
		}
	}
}
