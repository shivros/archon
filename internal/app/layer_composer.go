package app

import (
	"strings"

	xansi "github.com/charmbracelet/x/ansi"
)

type LayerOverlay struct {
	X     int
	Y     int
	Block string
}

type OverlayComposer interface {
	Compose(base string, overlays []LayerOverlay) string
}

type BlockJoiner interface {
	CombineHorizontal(left, right string, gap int) string
}

// LayerComposer is kept as a compatibility bridge for existing options.
type LayerComposer interface {
	OverlayComposer
	BlockJoiner
}

func WithOverlayComposer(composer OverlayComposer) ModelOption {
	return func(m *Model) {
		if m == nil || composer == nil {
			return
		}
		m.overlayComposer = composer
	}
}

func WithBlockJoiner(joiner BlockJoiner) ModelOption {
	return func(m *Model) {
		if m == nil || joiner == nil {
			return
		}
		m.overlayBlockJoiner = joiner
	}
}

func WithLayerComposer(composer LayerComposer) ModelOption {
	return func(m *Model) {
		if m == nil || composer == nil {
			return
		}
		m.overlayComposer = composer
		m.overlayBlockJoiner = composer
	}
}

type textLayerComposer struct{}

func NewTextLayerComposer() LayerComposer {
	return textLayerComposer{}
}

func NewTextOverlayComposer() OverlayComposer {
	return textLayerComposer{}
}

type defaultBlockJoiner struct{}

func NewDefaultBlockJoiner() BlockJoiner {
	return defaultBlockJoiner{}
}

func (textLayerComposer) Compose(base string, overlays []LayerOverlay) string {
	if base == "" || len(overlays) == 0 {
		return base
	}
	canvas := newTextCanvas(base)
	for _, overlay := range overlays {
		canvas.OverlayBlock(overlay.Block, overlay.X, overlay.Y)
	}
	return canvas.String()
}

func (textLayerComposer) CombineHorizontal(left, right string, gap int) string {
	return combineBlocks(left, right, gap)
}

func (defaultBlockJoiner) CombineHorizontal(left, right string, gap int) string {
	return combineBlocks(left, right, gap)
}

type textCanvas struct {
	lines []string
}

func newTextCanvas(text string) textCanvas {
	return textCanvas{lines: strings.Split(text, "\n")}
}

func (c *textCanvas) OverlayBlock(block string, x, y int) {
	if c == nil || block == "" || len(c.lines) == 0 {
		return
	}
	lines := strings.Split(block, "\n")
	for i := 0; i < len(lines); i++ {
		target := y + i
		if target < 0 || target >= len(c.lines) {
			continue
		}
		c.lines[target] = overlayLineAt(c.lines[target], lines[i], x)
	}
}

func overlayLineAt(base, overlay string, x int) string {
	if x < 0 {
		overlayWidth := xansi.StringWidth(overlay)
		if -x >= overlayWidth {
			return base
		}
		overlay = xansi.Cut(overlay, -x, overlayWidth)
		x = 0
	}
	overlayWidth := xansi.StringWidth(overlay)
	if overlayWidth <= 0 {
		return base
	}
	base = padToWidth(base, x+overlayWidth)
	baseWidth := xansi.StringWidth(base)
	prefix := xansi.Cut(base, 0, x)
	suffix := xansi.Cut(base, x+overlayWidth, baseWidth)
	return prefix + overlay + suffix
}

func (c *textCanvas) String() string {
	if c == nil {
		return ""
	}
	return strings.Join(c.lines, "\n")
}
