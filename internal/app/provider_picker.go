package app

import (
	"io"
	"strings"

	"control/internal/providers"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
)

type providerItem struct {
	id    string
	label string
}

func (p providerItem) Title() string {
	return p.label
}

func (p providerItem) Description() string {
	return ""
}

func (p providerItem) FilterValue() string {
	return p.id
}

type providerDelegate struct{}

func (d providerDelegate) Height() int  { return 1 }
func (d providerDelegate) Spacing() int { return 0 }
func (d providerDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd {
	return nil
}

func (d providerDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	entry, ok := item.(providerItem)
	if !ok {
		return
	}
	label := entry.Title()
	label = truncateToWidth(label, m.Width())
	style := sessionStyle
	if index == m.Index() {
		style = selectedStyle
	}
	io.WriteString(w, style.Render(label))
}

type ProviderPicker struct {
	list list.Model
}

func NewProviderPicker(width, height int) *ProviderPicker {
	items := []list.Item{}
	delegate := providerDelegate{}
	mlist := list.New(items, delegate, width, height)
	mlist.SetShowHelp(false)
	mlist.SetFilteringEnabled(false)
	mlist.SetShowPagination(false)
	mlist.SetShowStatusBar(false)
	mlist.Styles.Title = headerStyle
	return &ProviderPicker{list: mlist}
}

func (p *ProviderPicker) SetSize(width, height int) {
	p.list.SetSize(width, height)
}

func (p *ProviderPicker) Enter(selected string) {
	p.list.SetItems(defaultProviderItems())
	if selected != "" {
		for i, item := range p.list.Items() {
			entry, ok := item.(providerItem)
			if !ok {
				continue
			}
			if strings.EqualFold(entry.id, selected) {
				p.list.Select(i)
				return
			}
		}
	}
	p.list.Select(0)
}

func (p *ProviderPicker) View() string {
	return p.list.View()
}

func (p *ProviderPicker) SelectByRow(row int) bool {
	if row < 0 {
		return false
	}
	items := p.list.VisibleItems()
	if len(items) == 0 {
		return false
	}
	step := 1
	if step <= 0 {
		step = 1
	}
	perPage := p.list.Paginator.PerPage
	if perPage <= 0 {
		perPage = len(items)
	}
	start := p.list.Paginator.Page * perPage
	if start >= len(items) {
		start = 0
	}
	end := start + perPage - 1
	if end >= len(items) {
		end = len(items) - 1
	}
	pageIndex := row / step
	target := start + pageIndex
	if target > end {
		target = end
	}
	if target < 0 {
		target = 0
	}
	p.list.Select(target)
	return true
}

func (p *ProviderPicker) Scroll(lines int) bool {
	if lines == 0 {
		return false
	}
	steps := lines
	if steps < 0 {
		steps = -steps
	}
	for i := 0; i < steps; i++ {
		if lines < 0 {
			p.list.CursorUp()
		} else {
			p.list.CursorDown()
		}
	}
	return true
}

func (p *ProviderPicker) Move(delta int) {
	if delta < 0 {
		p.list.CursorUp()
	} else if delta > 0 {
		p.list.CursorDown()
	}
}

func (p *ProviderPicker) Selected() string {
	item := p.list.SelectedItem()
	entry, ok := item.(providerItem)
	if !ok {
		return ""
	}
	return entry.id
}

func defaultProviderItems() []list.Item {
	defs := providers.All()
	items := make([]list.Item, 0, len(defs))
	for _, def := range defs {
		items = append(items, providerItem{id: def.Name, label: def.Label})
	}
	return items
}
