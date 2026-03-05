package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

type sidebarSelectionIntentKind int

const (
	sidebarSelectionIntentNone sidebarSelectionIntentKind = iota
	sidebarSelectionIntentReplace
	sidebarSelectionIntentToggle
	sidebarSelectionIntentRangeAdd
)

type sidebarSelectionIntent struct {
	kind      sidebarSelectionIntentKind
	targetKey string
	anchorKey string
}

type SidebarSelectionReader interface {
	SelectedKeyCount() int
	SingleSelectedKey() string
	HasSelectedKeys() bool
	IsKeySelected(key string) bool
	SelectedKeys() []string
}

type SidebarSelectionController interface {
	SidebarSelectionReader
	SelectByKey(key string) bool
	ToggleFocusedSelection() bool
	ClearSelectedKeys() bool
	AddSelectionRangeByKeys(anchorKey, targetKey string) bool
}

type SidebarSelectionIntentPolicy interface {
	ResolveIntent(sidebar SidebarSelectionReader, clickedKey string, mouse tea.Mouse) sidebarSelectionIntent
}

type SidebarSelectionService interface {
	ApplyIntent(sidebar SidebarSelectionController, intent sidebarSelectionIntent) bool
}

type defaultSidebarSelectionIntentPolicy struct{}

func (defaultSidebarSelectionIntentPolicy) ResolveIntent(sidebar SidebarSelectionReader, clickedKey string, mouse tea.Mouse) sidebarSelectionIntent {
	clickedKey = strings.TrimSpace(clickedKey)
	if sidebar == nil || clickedKey == "" {
		return sidebarSelectionIntent{kind: sidebarSelectionIntentNone}
	}
	if mouse.Mod.Contains(tea.ModShift) && sidebar.SelectedKeyCount() == 1 {
		return sidebarSelectionIntent{
			kind:      sidebarSelectionIntentRangeAdd,
			targetKey: clickedKey,
			anchorKey: strings.TrimSpace(sidebar.SingleSelectedKey()),
		}
	}
	if mouse.Mod.Contains(tea.ModCtrl) {
		return sidebarSelectionIntent{
			kind:      sidebarSelectionIntentToggle,
			targetKey: clickedKey,
		}
	}
	return sidebarSelectionIntent{
		kind:      sidebarSelectionIntentReplace,
		targetKey: clickedKey,
	}
}

type defaultSidebarSelectionService struct{}

func (defaultSidebarSelectionService) ApplyIntent(sidebar SidebarSelectionController, intent sidebarSelectionIntent) bool {
	if sidebar == nil {
		return false
	}
	switch intent.kind {
	case sidebarSelectionIntentReplace:
		target := strings.TrimSpace(intent.targetKey)
		if target == "" {
			return false
		}
		if sidebar.HasSelectedKeys() && !sidebar.IsKeySelected(target) {
			_ = sidebar.ClearSelectedKeys()
		}
		return sidebar.SelectByKey(target)
	case sidebarSelectionIntentToggle:
		target := strings.TrimSpace(intent.targetKey)
		if target == "" || !sidebar.SelectByKey(target) {
			return false
		}
		return sidebar.ToggleFocusedSelection()
	case sidebarSelectionIntentRangeAdd:
		target := strings.TrimSpace(intent.targetKey)
		anchor := strings.TrimSpace(intent.anchorKey)
		if target == "" || anchor == "" {
			return false
		}
		if !sidebar.SelectByKey(target) {
			return false
		}
		_ = sidebar.AddSelectionRangeByKeys(anchor, target)
		return true
	default:
		return false
	}
}

func WithSidebarSelectionIntentPolicy(policy SidebarSelectionIntentPolicy) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if policy == nil {
			m.sidebarSelectionIntentPolicy = defaultSidebarSelectionIntentPolicy{}
			return
		}
		m.sidebarSelectionIntentPolicy = policy
	}
}

func WithSidebarSelectionService(service SidebarSelectionService) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if service == nil {
			m.sidebarSelectionService = defaultSidebarSelectionService{}
			return
		}
		m.sidebarSelectionService = service
	}
}

func (m *Model) sidebarSelectionIntentPolicyOrDefault() SidebarSelectionIntentPolicy {
	if m == nil || m.sidebarSelectionIntentPolicy == nil {
		return defaultSidebarSelectionIntentPolicy{}
	}
	return m.sidebarSelectionIntentPolicy
}

func (m *Model) sidebarSelectionServiceOrDefault() SidebarSelectionService {
	if m == nil || m.sidebarSelectionService == nil {
		return defaultSidebarSelectionService{}
	}
	return m.sidebarSelectionService
}
