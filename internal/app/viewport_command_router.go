package app

type ViewportCommandContext interface {
	DebugPanelNavigable() bool
	DebugPanelGotoTop() bool
	DebugPanelGotoBottom() bool
	TranscriptGotoTop()
	EnableFollow(value bool)
	PauseFollow(value bool)
}

type ViewportCommandRouter interface {
	RouteTop(ctx ViewportCommandContext) bool
	RouteBottom(ctx ViewportCommandContext) bool
}

type defaultViewportCommandRouter struct{}

func (defaultViewportCommandRouter) RouteTop(ctx ViewportCommandContext) bool {
	if ctx == nil {
		return false
	}
	if ctx.DebugPanelNavigable() {
		return ctx.DebugPanelGotoTop()
	}
	ctx.TranscriptGotoTop()
	ctx.PauseFollow(true)
	return true
}

func (defaultViewportCommandRouter) RouteBottom(ctx ViewportCommandContext) bool {
	if ctx == nil {
		return false
	}
	if ctx.DebugPanelNavigable() {
		return ctx.DebugPanelGotoBottom()
	}
	ctx.EnableFollow(true)
	return true
}

func WithViewportCommandRouter(router ViewportCommandRouter) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if router == nil {
			m.viewportCommandRouter = defaultViewportCommandRouter{}
			return
		}
		m.viewportCommandRouter = router
	}
}

func (m *Model) viewportCommandRouterOrDefault() ViewportCommandRouter {
	if m == nil || m.viewportCommandRouter == nil {
		return defaultViewportCommandRouter{}
	}
	return m.viewportCommandRouter
}

type modelViewportCommandContext struct {
	model *Model
}

func newModelViewportCommandContext(m *Model) ViewportCommandContext {
	return modelViewportCommandContext{model: m}
}

func (c modelViewportCommandContext) DebugPanelNavigable() bool {
	return c.model != nil && c.model.debugPanelNavigable()
}

func (c modelViewportCommandContext) DebugPanelGotoTop() bool {
	return c.model != nil && c.model.debugPanel != nil && c.model.debugPanel.GotoTop()
}

func (c modelViewportCommandContext) DebugPanelGotoBottom() bool {
	return c.model != nil && c.model.debugPanel != nil && c.model.debugPanel.GotoBottom()
}

func (c modelViewportCommandContext) TranscriptGotoTop() {
	if c.model == nil {
		return
	}
	c.model.viewport.GotoTop()
}

func (c modelViewportCommandContext) EnableFollow(value bool) {
	if c.model == nil {
		return
	}
	c.model.enableFollow(value)
}

func (c modelViewportCommandContext) PauseFollow(value bool) {
	if c.model == nil {
		return
	}
	c.model.pauseFollow(value)
}
