package app

import (
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
	glamouransi "github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
	xansi "github.com/charmbracelet/x/ansi"
)

var (
	rendererMu            sync.Mutex
	markdownDarkMode                              = true
	markdownRendererMaker markdownRendererFactory = glamourMarkdownRendererFactory{}
)

func renderMarkdown(input string, width int) string {
	input = strings.TrimRight(input, "\n")
	if input == "" {
		return ""
	}
	if width <= 0 {
		width = 80
	}
	renderer, err := markdownRendererFactoryOrDefault().New(width, markdownBackgroundDark())
	if err != nil || renderer == nil {
		return input
	}
	out, err := renderer.Render(input)
	if err != nil {
		return input
	}
	out = strings.TrimRight(out, "\n")
	out = xansi.Hardwrap(out, width, true)
	return strings.TrimRight(out, "\n")
}

type markdownRenderer interface {
	Render(input string) (string, error)
}

type markdownRendererFactory interface {
	New(width int, dark bool) (markdownRenderer, error)
}

type glamourMarkdownRenderer struct {
	renderer *glamour.TermRenderer
}

func (r glamourMarkdownRenderer) Render(input string) (string, error) {
	if r.renderer == nil {
		return "", nil
	}
	return r.renderer.Render(input)
}

type glamourMarkdownRendererFactory struct{}

func (glamourMarkdownRendererFactory) New(width int, dark bool) (markdownRenderer, error) {
	if width <= 0 {
		width = 80
	}
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStyles(buildStyleConfig(dark)),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return nil, err
	}
	return glamourMarkdownRenderer{renderer: renderer}, nil
}

func markdownBackgroundDark() bool {
	rendererMu.Lock()
	defer rendererMu.Unlock()
	return markdownDarkMode
}

func setMarkdownBackgroundDark(dark bool) bool {
	rendererMu.Lock()
	defer rendererMu.Unlock()
	changed := markdownDarkMode != dark
	markdownDarkMode = dark
	return changed
}

func markdownRendererFactoryOrDefault() markdownRendererFactory {
	rendererMu.Lock()
	defer rendererMu.Unlock()
	if markdownRendererMaker == nil {
		markdownRendererMaker = glamourMarkdownRendererFactory{}
	}
	return markdownRendererMaker
}

func buildStyleConfig(dark bool) glamouransi.StyleConfig {
	var base glamouransi.StyleConfig
	if dark {
		base = styles.DarkStyleConfig
	} else {
		base = styles.LightStyleConfig
	}
	// Keep bubble spacing controlled by lipgloss padding instead of Glamour's
	// document-level prefix/suffix newlines and side margins.
	base.Document.BlockPrefix = ""
	base.Document.BlockSuffix = ""
	zero := uint(0)
	base.Document.Margin = &zero
	faint := true
	color := markdownBlockQuoteColor
	base.BlockQuote.Faint = &faint
	base.BlockQuote.Color = &color
	return base
}

func escapeMarkdown(text string) string {
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		line = strings.ReplaceAll(line, "`", "\\`")
		trimmed := strings.TrimLeft(line, " \t")
		prefix := line[:len(line)-len(trimmed)]
		switch {
		case strings.HasPrefix(trimmed, "#"),
			strings.HasPrefix(trimmed, ">"),
			strings.HasPrefix(trimmed, "- "),
			strings.HasPrefix(trimmed, "* "),
			strings.HasPrefix(trimmed, "+ "):
			lines[i] = prefix + "\\" + trimmed
		case isNumberedList(trimmed):
			lines[i] = prefix + "\\" + trimmed
		default:
			lines[i] = prefix + trimmed
		}
	}
	return strings.Join(lines, "\n")
}

func isNumberedList(text string) bool {
	dot := strings.IndexByte(text, '.')
	if dot <= 0 {
		return false
	}
	if dot+1 >= len(text) || text[dot+1] != ' ' {
		return false
	}
	for i := 0; i < dot; i++ {
		if text[i] < '0' || text[i] > '9' {
			return false
		}
	}
	return true
}
