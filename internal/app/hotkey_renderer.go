package app

import (
	"sort"
	"strings"
)

type HotkeyRenderer struct {
	hotkeys  []Hotkey
	resolver HotkeyResolver
}

func NewHotkeyRenderer(hotkeys []Hotkey, resolver HotkeyResolver) *HotkeyRenderer {
	return &HotkeyRenderer{hotkeys: hotkeys, resolver: resolver}
}

func (r *HotkeyRenderer) Render(m *Model) string {
	if r == nil {
		return ""
	}
	contexts := map[HotkeyContext]struct{}{}
	if r.resolver != nil {
		for _, ctx := range r.resolver.ActiveContexts(m) {
			contexts[ctx] = struct{}{}
		}
	}
	var ctxList []HotkeyContext
	for ctx := range contexts {
		ctxList = append(ctxList, ctx)
	}
	visible := FilterHotkeys(r.hotkeys, ctxList)
	parts := make([]string, 0, len(visible))
	for _, hk := range visible {
		parts = append(parts, hk.Key+" "+hk.Label)
	}
	return strings.Join(parts, " • ")
}

func FilterHotkeys(hotkeys []Hotkey, contexts []HotkeyContext) []Hotkey {
	if len(hotkeys) == 0 || len(contexts) == 0 {
		return nil
	}
	allowed := map[HotkeyContext]struct{}{}
	for _, ctx := range contexts {
		allowed[ctx] = struct{}{}
	}
	var out []Hotkey
	for _, hk := range hotkeys {
		if _, ok := allowed[hk.Context]; ok {
			out = append(out, hk)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Priority == out[j].Priority {
			return out[i].Key < out[j].Key
		}
		return out[i].Priority < out[j].Priority
	})
	return out
}
