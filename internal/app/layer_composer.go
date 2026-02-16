package app

import (
	"strings"
)

type LayerOverlay struct {
	Row   int
	Block string
}

type LayerComposer interface {
	Compose(base string, overlays []LayerOverlay) string
	CombineHorizontal(left, right string, gap int) string
}

func WithLayerComposer(composer LayerComposer) ModelOption {
	return func(m *Model) {
		if m == nil || composer == nil {
			return
		}
		m.layerComposer = composer
	}
}

type textLayerComposer struct{}

func NewTextLayerComposer() LayerComposer {
	return textLayerComposer{}
}

func (textLayerComposer) Compose(base string, overlays []LayerOverlay) string {
	if base == "" || len(overlays) == 0 {
		return base
	}
	canvas := newTextCanvas(base)
	for _, overlay := range overlays {
		canvas.OverlayBlock(overlay.Block, overlay.Row)
	}
	return canvas.String()
}

func (textLayerComposer) CombineHorizontal(left, right string, gap int) string {
	return combineBlocks(left, right, gap)
}

type textCanvas struct {
	lines []string
}

func newTextCanvas(text string) textCanvas {
	return textCanvas{lines: strings.Split(text, "\n")}
}

func (c *textCanvas) OverlayBlock(block string, row int) {
	if c == nil || row < 0 || block == "" || len(c.lines) == 0 {
		return
	}
	lines := strings.Split(block, "\n")
	for i := 0; i < len(lines); i++ {
		target := row + i
		if target < 0 || target >= len(c.lines) {
			continue
		}
		c.lines[target] = lines[i]
	}
}

func (c *textCanvas) String() string {
	if c == nil {
		return ""
	}
	return strings.Join(c.lines, "\n")
}
