package app

import "strings"

type TransientOverlayContext struct {
	Body       string
	BodyHeight int
}

type TransientOverlayProvider interface {
	Build(*Model, TransientOverlayContext) (LayerOverlay, bool)
}

func WithTransientOverlayProviders(providers []TransientOverlayProvider) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if len(providers) == 0 {
			m.transientOverlayProviders = nil
			return
		}
		next := make([]TransientOverlayProvider, 0, len(providers))
		for _, provider := range providers {
			if provider == nil {
				continue
			}
			next = append(next, provider)
		}
		m.transientOverlayProviders = next
	}
}

func defaultTransientOverlayProviders() []TransientOverlayProvider {
	return []TransientOverlayProvider{
		menuBarOverlayProvider{},
		menuDropdownOverlayProvider{},
		contextMenuOverlayProvider{},
		confirmOverlayProvider{},
		composeOptionPickerOverlayProvider{},
		statusHistoryOverlayProvider{},
		settingsMenuOverlayProvider{},
		toastOverlayProvider{},
	}
}

func (m *Model) overlayComposerOrDefault() OverlayComposer {
	if m == nil || m.overlayComposer == nil {
		return NewTextOverlayComposer()
	}
	return m.overlayComposer
}

func (m *Model) overlayBlockJoinerOrDefault() BlockJoiner {
	if m == nil || m.overlayBlockJoiner == nil {
		return NewDefaultBlockJoiner()
	}
	return m.overlayBlockJoiner
}

func (m *Model) transientOverlayProvidersOrDefault() []TransientOverlayProvider {
	if m == nil || len(m.transientOverlayProviders) == 0 {
		return defaultTransientOverlayProviders()
	}
	return m.transientOverlayProviders
}

type menuBarOverlayProvider struct{}

func (menuBarOverlayProvider) Build(m *Model, _ TransientOverlayContext) (LayerOverlay, bool) {
	if m == nil || m.menu == nil {
		return LayerOverlay{}, false
	}
	menuBar := m.menu.MenuBarView(m.width)
	if menuBar == "" {
		return LayerOverlay{}, false
	}
	return LayerOverlay{X: 0, Y: 0, Block: menuBar}, true
}

type menuDropdownOverlayProvider struct{}

func (menuDropdownOverlayProvider) Build(m *Model, _ TransientOverlayContext) (LayerOverlay, bool) {
	if m == nil || m.menu == nil || !m.menu.IsDropdownOpen() {
		return LayerOverlay{}, false
	}
	menuDrop := m.menu.DropdownView(m.sidebarWidth())
	if menuDrop == "" {
		return LayerOverlay{}, false
	}
	block := menuDrop
	if m.menu.HasSubmenu() {
		submenu := m.menu.SubmenuView(0)
		block = m.overlayBlockJoinerOrDefault().CombineHorizontal(menuDrop, submenu, 1)
	}
	return LayerOverlay{X: 0, Y: 1, Block: block}, true
}

type contextMenuOverlayProvider struct{}

func (contextMenuOverlayProvider) Build(m *Model, ctx TransientOverlayContext) (LayerOverlay, bool) {
	if m == nil || m.contextMenu == nil || !m.contextMenu.IsOpen() {
		return LayerOverlay{}, false
	}
	menuBlock, x, y := m.contextMenu.ViewBlock(m.width, ctx.BodyHeight)
	if menuBlock == "" {
		return LayerOverlay{}, false
	}
	return LayerOverlay{X: x, Y: y, Block: menuBlock}, true
}

type confirmOverlayProvider struct{}

func (confirmOverlayProvider) Build(m *Model, ctx TransientOverlayContext) (LayerOverlay, bool) {
	if m == nil || m.confirm == nil || !m.confirm.IsOpen() {
		return LayerOverlay{}, false
	}
	confirmBlock, x, y := m.confirm.ViewBlock(m.width, ctx.BodyHeight)
	if confirmBlock == "" {
		return LayerOverlay{}, false
	}
	return LayerOverlay{X: x, Y: y, Block: confirmBlock}, true
}

type composeOptionPickerOverlayProvider struct{}

func (composeOptionPickerOverlayProvider) Build(m *Model, _ TransientOverlayContext) (LayerOverlay, bool) {
	if m == nil {
		return LayerOverlay{}, false
	}
	popup, x, y := m.composeOptionPopupPlacement()
	if popup == "" {
		return LayerOverlay{}, false
	}
	return LayerOverlay{X: x, Y: y, Block: popup}, true
}

type statusHistoryOverlayProvider struct{}

func (statusHistoryOverlayProvider) Build(m *Model, ctx TransientOverlayContext) (LayerOverlay, bool) {
	if m == nil {
		return LayerOverlay{}, false
	}
	historyDrop, x, y, ok := m.statusHistoryOverlayView(ctx.BodyHeight)
	if !ok {
		return LayerOverlay{}, false
	}
	return LayerOverlay{X: x, Y: y, Block: historyDrop}, true
}

type settingsMenuOverlayProvider struct{}

func (settingsMenuOverlayProvider) Build(m *Model, ctx TransientOverlayContext) (LayerOverlay, bool) {
	if m == nil || m.settingsMenu == nil || !m.settingsMenu.IsOpen() {
		return LayerOverlay{}, false
	}
	helpMappings := m.settingsMenuHotkeyCatalogOrDefault().Mappings(m.settingsMenuHotkeySource())
	presenter := m.settingsMenuPresenter
	if presenter == nil {
		presenter = defaultSettingsMenuPresenter{}
	}
	settingsBlock, x, y := presenter.View(m.settingsMenu, m.width, ctx.BodyHeight, helpMappings)
	if settingsBlock == "" {
		return LayerOverlay{}, false
	}
	return LayerOverlay{X: x, Y: y, Block: settingsBlock}, true
}

type toastOverlayProvider struct{}

func (toastOverlayProvider) Build(m *Model, ctx TransientOverlayContext) (LayerOverlay, bool) {
	if m == nil {
		return LayerOverlay{}, false
	}
	line, row, ok := m.toastOverlay(ctx.BodyHeight)
	if !ok || strings.TrimSpace(line) == "" {
		return LayerOverlay{}, false
	}
	return LayerOverlay{X: 0, Y: row, Block: line}, true
}
