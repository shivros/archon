package app

import "control/internal/providers"

type ProviderPicker struct {
	picker *SelectPicker
}

func NewProviderPicker(width, height int) *ProviderPicker {
	return &ProviderPicker{picker: NewSelectPicker(width, height)}
}

func (p *ProviderPicker) SetSize(width, height int) {
	if p == nil || p.picker == nil {
		return
	}
	p.picker.SetSize(width, height)
}

func (p *ProviderPicker) Enter(selected string) {
	if p == nil || p.picker == nil {
		return
	}
	options := defaultProviderItems()
	p.picker.SetQuery("")
	p.picker.SetOptions(options)
	if selected != "" && p.picker.SelectID(selected) {
		return
	}
	if len(options) > 0 {
		p.picker.SelectID(options[0].id)
	}
}

func (p *ProviderPicker) View() string {
	if p == nil || p.picker == nil {
		return ""
	}
	return p.picker.View()
}

func (p *ProviderPicker) SelectByRow(row int) bool {
	if p == nil || p.picker == nil {
		return false
	}
	return p.picker.HandleClick(row)
}

func (p *ProviderPicker) Scroll(lines int) bool {
	if p == nil || p.picker == nil {
		return false
	}
	return p.picker.Move(lines)
}

func (p *ProviderPicker) Move(delta int) {
	if p == nil || p.picker == nil {
		return
	}
	p.picker.Move(delta)
}

func (p *ProviderPicker) Selected() string {
	if p == nil || p.picker == nil {
		return ""
	}
	return p.picker.SelectedID()
}

func (p *ProviderPicker) Query() string {
	if p == nil || p.picker == nil {
		return ""
	}
	return p.picker.Query()
}

func (p *ProviderPicker) AppendQuery(text string) bool {
	if p == nil || p.picker == nil {
		return false
	}
	return p.picker.AppendQuery(text)
}

func (p *ProviderPicker) BackspaceQuery() bool {
	if p == nil || p.picker == nil {
		return false
	}
	return p.picker.BackspaceQuery()
}

func (p *ProviderPicker) ClearQuery() bool {
	if p == nil || p.picker == nil {
		return false
	}
	return p.picker.ClearQuery()
}

func defaultProviderItems() []selectOption {
	defs := providers.All()
	items := make([]selectOption, 0, len(defs))
	for _, def := range defs {
		items = append(items, selectOption{
			id:     def.Name,
			label:  def.Label,
			search: def.Name + " " + def.Label,
		})
	}
	return items
}
