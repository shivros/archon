package app

import (
	"strings"

	"charm.land/bubbles/v2/viewport"
)

const debugPanelWaitingMessage = "Waiting for debug stream..."

type DebugPanelHeaderRenderer interface {
	RenderHeader(title string) string
}

type defaultDebugPanelHeaderRenderer struct{}

func (defaultDebugPanelHeaderRenderer) RenderHeader(title string) string {
	return headerStyle.Render(title)
}

type DebugPanelController struct {
	viewport     viewport.Model
	renderer     DebugPanelHeaderRenderer
	header       string
	content      string
	cachedView   string
	cachedHeight int
	dirty        bool
}

func NewDebugPanelController(width, height int, renderer DebugPanelHeaderRenderer) *DebugPanelController {
	if renderer == nil {
		renderer = defaultDebugPanelHeaderRenderer{}
	}
	vp := viewport.New(viewport.WithWidth(max(1, width)), viewport.WithHeight(max(1, height)))
	vp.SoftWrap = true
	vp.SetContent(debugPanelWaitingMessage)
	return &DebugPanelController{
		viewport:     vp,
		renderer:     renderer,
		header:       "Debug",
		content:      debugPanelWaitingMessage,
		cachedHeight: 2,
		dirty:        true,
	}
}

func (c *DebugPanelController) ScrollUp(lines int) bool {
	if c == nil || lines <= 0 {
		return false
	}
	before := c.viewport.YOffset()
	c.viewport.ScrollUp(lines)
	if c.viewport.YOffset() == before {
		return false
	}
	c.dirty = true
	return true
}

func (c *DebugPanelController) ScrollDown(lines int) bool {
	if c == nil || lines <= 0 {
		return false
	}
	before := c.viewport.YOffset()
	c.viewport.ScrollDown(lines)
	if c.viewport.YOffset() == before {
		return false
	}
	c.dirty = true
	return true
}

func (c *DebugPanelController) PageUp() bool {
	if c == nil {
		return false
	}
	before := c.viewport.YOffset()
	c.viewport.PageUp()
	if c.viewport.YOffset() == before {
		return false
	}
	c.dirty = true
	return true
}

func (c *DebugPanelController) PageDown() bool {
	if c == nil {
		return false
	}
	before := c.viewport.YOffset()
	c.viewport.PageDown()
	if c.viewport.YOffset() == before {
		return false
	}
	c.dirty = true
	return true
}

func (c *DebugPanelController) ScrollLeft(cols int) bool {
	if c == nil || cols <= 0 {
		return false
	}
	before := c.viewport.XOffset()
	c.viewport.ScrollLeft(cols)
	if c.viewport.XOffset() == before {
		return false
	}
	c.dirty = true
	return true
}

func (c *DebugPanelController) ScrollRight(cols int) bool {
	if c == nil || cols <= 0 {
		return false
	}
	before := c.viewport.XOffset()
	c.viewport.ScrollRight(cols)
	if c.viewport.XOffset() == before {
		return false
	}
	c.dirty = true
	return true
}

func (c *DebugPanelController) GotoTop() bool {
	if c == nil {
		return false
	}
	before := c.viewport.YOffset()
	c.viewport.GotoTop()
	if c.viewport.YOffset() == before {
		return false
	}
	c.dirty = true
	return true
}

func (c *DebugPanelController) GotoBottom() bool {
	if c == nil {
		return false
	}
	before := c.viewport.YOffset()
	c.viewport.GotoBottom()
	if c.viewport.YOffset() == before {
		return false
	}
	c.dirty = true
	return true
}

func (c *DebugPanelController) Height() int {
	if c == nil {
		return 0
	}
	return c.viewport.Height()
}

func (c *DebugPanelController) YOffset() int {
	if c == nil {
		return 0
	}
	return c.viewport.YOffset()
}

func (c *DebugPanelController) Resize(width, height int) {
	if c == nil {
		return
	}
	nextWidth := max(1, width)
	nextHeight := max(1, height)
	if c.viewport.Width() == nextWidth && c.viewport.Height() == nextHeight {
		return
	}
	c.viewport.SetWidth(nextWidth)
	c.viewport.SetHeight(nextHeight)
	c.dirty = true
}

func (c *DebugPanelController) SetContent(content string) {
	if c == nil {
		return
	}
	if strings.TrimSpace(content) == "" {
		content = debugPanelWaitingMessage
	}
	if c.content == content {
		return
	}
	c.content = content
	c.viewport.SetContent(content)
	c.dirty = true
}

func (c *DebugPanelController) View() (string, int) {
	if c == nil {
		return "", 0
	}
	if c.dirty {
		body := c.viewport.View()
		if strings.TrimSpace(body) == "" {
			body = debugPanelWaitingMessage
		}
		c.cachedView = c.renderer.RenderHeader(c.header) + "\n" + body
		c.cachedHeight = blockLineCount(c.cachedView)
		c.dirty = false
	}
	return c.cachedView, c.cachedHeight
}

func blockLineCount(text string) int {
	if text == "" {
		return 0
	}
	return strings.Count(text, "\n") + 1
}
