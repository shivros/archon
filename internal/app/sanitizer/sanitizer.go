package sanitizer

type InputSanitizer interface {
	Sanitize(input string) string
}

type Config struct {
	AllowNewlines      bool
	AllowTabs          bool
	ReplaceNewlineWith string
	MaxLength          int
	CustomPatterns     []*EscapePattern
}

type TerminalSanitizer struct {
	config         Config
	escapePatterns []*EscapePattern
}

func NewTerminalSanitizer(config Config) *TerminalSanitizer {
	patterns := make([]*EscapePattern, len(AllEscapePatterns))
	copy(patterns, AllEscapePatterns)
	patterns = append(patterns, config.CustomPatterns...)

	return &TerminalSanitizer{
		config:         config,
		escapePatterns: patterns,
	}
}

func DefaultConfig() Config {
	return Config{
		AllowNewlines:  true,
		AllowTabs:      false,
		MaxLength:      0,
		CustomPatterns: nil,
	}
}

func SingleLineConfig() Config {
	return Config{
		AllowNewlines:      false,
		ReplaceNewlineWith: " ",
		AllowTabs:          false,
		MaxLength:          0,
		CustomPatterns:     nil,
	}
}

func (s *TerminalSanitizer) Sanitize(input string) string {
	if input == "" {
		return input
	}

	for _, p := range s.escapePatterns {
		input = p.Pattern.ReplaceAllString(input, "")
	}

	var result []rune
	for _, r := range input {
		kept, replacement := s.shouldKeep(r)
		if kept {
			result = append(result, r)
		} else if replacement != "" {
			result = append(result, []rune(replacement)...)
		}
	}

	sanitized := string(result)

	if s.config.MaxLength > 0 && len(sanitized) > s.config.MaxLength {
		sanitized = sanitized[:s.config.MaxLength]
	}

	return sanitized
}

func (s *TerminalSanitizer) shouldKeep(r rune) (bool, string) {
	switch {
	case r == '\n':
		if s.config.AllowNewlines {
			return true, ""
		}
		return false, s.config.ReplaceNewlineWith
	case r == '\t':
		return s.config.AllowTabs, ""
	case r < 32 || r == 127:
		return false, ""
	default:
		return true, ""
	}
}

type ChainedSanitizer struct {
	sanitizers []InputSanitizer
}

func NewChainedSanitizer(sanitizers ...InputSanitizer) *ChainedSanitizer {
	return &ChainedSanitizer{sanitizers: sanitizers}
}

func (c *ChainedSanitizer) Sanitize(input string) string {
	for _, s := range c.sanitizers {
		input = s.Sanitize(input)
	}
	return input
}

type NopSanitizer struct{}

func NewNopSanitizer() *NopSanitizer {
	return &NopSanitizer{}
}

func (n *NopSanitizer) Sanitize(input string) string {
	return input
}
