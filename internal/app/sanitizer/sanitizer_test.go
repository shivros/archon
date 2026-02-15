package sanitizer

import (
	"regexp"
	"strings"
	"testing"
)

func TestCSIEscapePattern(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "mouse scroll SGR sequence",
			input:    "hello\x1b[<65;113;33Mworld",
			expected: "helloworld",
		},
		{
			name:     "multiple mouse sequences",
			input:    "\x1b[<65;113;33M\x1b[<65;113;33M\x1b[<65;113;33Mtext",
			expected: "text",
		},
		{
			name:     "ANSI color code",
			input:    "\x1b[31mred text\x1b[0m",
			expected: "red text",
		},
		{
			name:     "cursor position",
			input:    "\x1b[10;20Hposition",
			expected: "position",
		},
		{
			name:     "cursor up",
			input:    "\x1b[5Aup",
			expected: "up",
		},
		{
			name:     "private mode set",
			input:    "\x1b[?25hvisible",
			expected: "visible",
		},
		{
			name:     "private mode reset",
			input:    "\x1b[?25linvisible",
			expected: "invisible",
		},
		{
			name:     "SGR mouse release",
			input:    "\x1b[<0;100;50mclick",
			expected: "click",
		},
		{
			name:     "dec private mode",
			input:    "\x1b[?1hset",
			expected: "set",
		},
		{
			name:     "linux private mode",
			input:    "\x1b[>4;0mmode",
			expected: "mode",
		},
		{
			name:     "normal text unchanged",
			input:    "Hello, World!",
			expected: "Hello, World!",
		},
		{
			name:     "unicode preserved",
			input:    "日本語テスト",
			expected: "日本語テスト",
		},
		{
			name:     "real world mouse scroll example",
			input:    strings.Repeat("\x1b[<65;113;33M", 26) + "user message",
			expected: "user message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CSIEscapePattern.Pattern.ReplaceAllString(tt.input, "")
			if got != tt.expected {
				t.Errorf("CSIEscapePattern.ReplaceAllString(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestOSCescapePattern(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "OSC window title with BEL",
			input:    "\x1b]0;My Title\x07text",
			expected: "text",
		},
		{
			name:     "OSC window title with ST",
			input:    "\x1b]2;Title\x1b\\text",
			expected: "text",
		},
		{
			name:     "OSC hyperlink",
			input:    "\x1b]8;;http://example.com\x07link\x1b]8;;\x07",
			expected: "link",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := OSCescapePattern.Pattern.ReplaceAllString(tt.input, "")
			if got != tt.expected {
				t.Errorf("OSCescapePattern.ReplaceAllString(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestRemoveEscapeSequences(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "mixed escape sequences",
			input:    "\x1b[31m\x1b]0;Title\x07\x1b[<65;113;33Mtext",
			expected: "text",
		},
		{
			name:     "real world scroll in compose",
			input:    "user typed: " + strings.Repeat("\x1b[<65;113;33M", 10) + "hello",
			expected: "user typed: hello",
		},
		{
			name:     "orphaned mouse pattern without ESC",
			input:    "[<65;155;38M[<65;155;38Mtext",
			expected: "text",
		},
		{
			name:     "orphaned mouse pattern lowercase m",
			input:    "[<65;151;37m[<65;151;37mhello",
			expected: "hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RemoveEscapeSequences(tt.input)
			if got != tt.expected {
				t.Errorf("RemoveEscapeSequences(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestTerminalSanitizer_Sanitize(t *testing.T) {
	tests := []struct {
		name     string
		config   Config
		input    string
		expected string
	}{
		{
			name:     "default config keeps newlines",
			config:   DefaultConfig(),
			input:    "line1\nline2",
			expected: "line1\nline2",
		},
		{
			name:     "single line config replaces newlines with space",
			config:   SingleLineConfig(),
			input:    "line1\nline2",
			expected: "line1 line2",
		},
		{
			name:     "removes control characters",
			config:   DefaultConfig(),
			input:    "hello\x00world\x1ftest",
			expected: "helloworldtest",
		},
		{
			name:     "removes DEL character",
			config:   DefaultConfig(),
			input:    "hello\x7fworld",
			expected: "helloworld",
		},
		{
			name:     "removes mouse sequences",
			config:   DefaultConfig(),
			input:    "\x1b[<65;113;33Mhello",
			expected: "hello",
		},
		{
			name:     "preserves unicode",
			config:   DefaultConfig(),
			input:    "日本語\n中文\n한국어",
			expected: "日本語\n中文\n한국어",
		},
		{
			name:     "max length truncation",
			config:   Config{MaxLength: 5},
			input:    "hello world",
			expected: "hello",
		},
		{
			name:     "custom pattern",
			config:   Config{CustomPatterns: []*EscapePattern{{Name: "custom", Pattern: regexp.MustCompile(`FOO`)}}},
			input:    "FOObarFOO",
			expected: "bar",
		},
		{
			name:     "empty input",
			config:   DefaultConfig(),
			input:    "",
			expected: "",
		},
		{
			name:     "tabs removed by default",
			config:   DefaultConfig(),
			input:    "col1\tcol2",
			expected: "col1col2",
		},
		{
			name:     "tabs kept when allowed",
			config:   Config{AllowTabs: true},
			input:    "col1\tcol2",
			expected: "col1\tcol2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewTerminalSanitizer(tt.config)
			got := s.Sanitize(tt.input)
			if got != tt.expected {
				t.Errorf("Sanitize(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestChainedSanitizer(t *testing.T) {
	s1 := NewTerminalSanitizer(DefaultConfig())
	s2 := NewTerminalSanitizer(Config{MaxLength: 5})

	chained := NewChainedSanitizer(s1, s2)

	input := "\x1b[<65;113;33Mhello world"
	expected := "hello"

	got := chained.Sanitize(input)
	if got != expected {
		t.Errorf("ChainedSanitizer.Sanitize(%q) = %q, want %q", input, got, expected)
	}
}

func TestNopSanitizer(t *testing.T) {
	s := NewNopSanitizer()
	input := "\x1b[31mhello\x1b[0m"

	got := s.Sanitize(input)
	if got != input {
		t.Errorf("NopSanitizer.Sanitize(%q) = %q, want %q", input, got, input)
	}
}

func BenchmarkTerminalSanitizer_Sanitize(b *testing.B) {
	input := strings.Repeat("\x1b[<65;113;33M", 100) + "hello world\n" + strings.Repeat("test ", 100)
	s := NewTerminalSanitizer(DefaultConfig())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Sanitize(input)
	}
}

func BenchmarkRemoveEscapeSequences(b *testing.B) {
	input := strings.Repeat("\x1b[<65;113;33M", 100) + "hello world"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		RemoveEscapeSequences(input)
	}
}
