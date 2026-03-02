package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

type defaultHighlightCoordinator struct {
	modeSource highlightModeSource
	policy     highlightContextPolicy
	surfaces   []highlightSurface
	state      highlightState
}

func NewDefaultHighlightCoordinator(modeSource highlightModeSource, policy highlightContextPolicy, surfaces ...highlightSurface) highlightCoordinator {
	if policy == nil {
		policy = NewDefaultHighlightContextPolicy()
	}
	return &defaultHighlightCoordinator{
		modeSource: modeSource,
		policy:     policy,
		surfaces:   append([]highlightSurface(nil), surfaces...),
	}
}

func (c *defaultHighlightCoordinator) Begin(msg tea.MouseMsg, layout mouseLayout) bool {
	if c == nil {
		return false
	}
	mode := c.currentMode()
	for _, surface := range c.surfaces {
		if surface == nil {
			continue
		}
		ctx := surface.Context()
		if !c.policyAllows(mode, ctx) {
			continue
		}
		point, ok := surface.PointFromMouse(msg, layout)
		if !ok {
			continue
		}
		c.state = highlightState{
			Active:  true,
			Context: ctx,
			Anchor:  point,
			Focus:   point,
		}
		return true
	}
	return false
}

func (c *defaultHighlightCoordinator) Update(msg tea.MouseMsg, layout mouseLayout) bool {
	if c == nil || !c.state.Active {
		return false
	}
	if !c.policyAllows(c.currentMode(), c.state.Context) {
		return false
	}
	surface := c.surfaceForContext(c.state.Context)
	if surface == nil {
		return false
	}
	point, ok := surface.PointFromMouse(msg, layout)
	if !ok {
		return false
	}
	changed := point.BlockIndex != c.state.Focus.BlockIndex ||
		point.SidebarRow != c.state.Focus.SidebarRow ||
		strings.TrimSpace(point.SidebarKey) != strings.TrimSpace(c.state.Focus.SidebarKey)
	if !changed {
		return false
	}
	c.state.Focus = point
	c.state.Dragging = true
	rangeState, ok := surface.RangeFromPoints(c.state.Anchor, c.state.Focus)
	if !ok {
		c.state.Range = highlightRange{}
		return true
	}
	rangeState.Context = c.state.Context
	rangeState.HasSelection = true
	c.state.Range = rangeState
	return true
}

func (c *defaultHighlightCoordinator) End(msg tea.MouseMsg, layout mouseLayout) bool {
	if c == nil || !c.state.Active {
		return false
	}
	if c.state.Dragging {
		_ = c.Update(msg, layout)
		c.state.Active = false
		return true
	}
	c.state = highlightState{}
	return false
}

func (c *defaultHighlightCoordinator) Clear() bool {
	if c == nil {
		return false
	}
	if !c.state.Active && !c.state.Range.HasSelection && c.state.Context == highlightContextNone {
		return false
	}
	c.state = highlightState{}
	return true
}

func (c *defaultHighlightCoordinator) State() highlightState {
	if c == nil {
		return highlightState{}
	}
	out := c.state
	if len(c.state.Range.SidebarKeys) > 0 {
		out.Range.SidebarKeys = cloneStringSet(c.state.Range.SidebarKeys)
	}
	return out
}

func (c *defaultHighlightCoordinator) surfaceForContext(ctx highlightContext) highlightSurface {
	for _, surface := range c.surfaces {
		if surface == nil {
			continue
		}
		if surface.Context() == ctx {
			return surface
		}
	}
	return nil
}

func (c *defaultHighlightCoordinator) currentMode() uiMode {
	if c == nil || c.modeSource == nil {
		return uiModeNormal
	}
	return c.modeSource.CurrentUIMode()
}

func (c *defaultHighlightCoordinator) policyAllows(mode uiMode, ctx highlightContext) bool {
	if c == nil || c.policy == nil {
		return false
	}
	return c.policy.AllowsContext(mode, ctx)
}
