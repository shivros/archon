package sanitizer

import "regexp"

type EscapePattern struct {
	Name    string
	Pattern *regexp.Regexp
}

var CSIEscapePattern = &EscapePattern{
	Name:    "CSI",
	Pattern: regexp.MustCompile(`\x1b\[[<>?=]?[0-9;]*[A-Za-z@^` + "`" + `~{|}!]`),
}

var OSCescapePattern = &EscapePattern{
	Name:    "OSC",
	Pattern: regexp.MustCompile(`\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)`),
}

var OtherEscapePattern = &EscapePattern{
	Name:    "Other",
	Pattern: regexp.MustCompile(`\x1b[()][AB012]`),
}

var OrphanedMousePattern = &EscapePattern{
	Name:    "OrphanedMouse",
	Pattern: regexp.MustCompile(`\[<[0-9]+;[0-9]+;[0-9]+[Mm]`),
}

var AllEscapePatterns = []*EscapePattern{
	CSIEscapePattern,
	OSCescapePattern,
	OtherEscapePattern,
	OrphanedMousePattern,
}

func RemoveEscapeSequences(input string) string {
	for _, p := range AllEscapePatterns {
		input = p.Pattern.ReplaceAllString(input, "")
	}
	return input
}
