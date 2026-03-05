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
	// ResolveIntent maps a click gesture to a selection intent.
	ResolveIntent(sidebar SidebarSelectionReader, clickedKey string, mouse tea.Mouse) sidebarSelectionIntent
}

type SidebarSelectionRangeAnchorPolicy interface {
	// ResolveAnchor returns the anchor key for range selection.
	// Returning an empty key means range selection is not available.
	ResolveAnchor(sidebar SidebarSelectionReader, clickedKey string, mouse tea.Mouse) string
}

type SidebarSelectionService interface {
	// ApplyIntent mutates selection state and returns true when selection state changed.
	ApplyIntent(sidebar SidebarSelectionController, intent sidebarSelectionIntent) bool
}

type sidebarSelectionRangeAnchorAware interface {
	WithRangeAnchorPolicy(policy SidebarSelectionRangeAnchorPolicy) SidebarSelectionIntentPolicy
}

type defaultSidebarSelectionIntentPolicy struct {
	rangeAnchorPolicy SidebarSelectionRangeAnchorPolicy
}

type singleSelectedSidebarRangeAnchorPolicy struct{}

func (singleSelectedSidebarRangeAnchorPolicy) ResolveAnchor(sidebar SidebarSelectionReader, _ string, _ tea.Mouse) string {
	if sidebar == nil || sidebar.SelectedKeyCount() != 1 {
		return ""
	}
	return strings.TrimSpace(sidebar.SingleSelectedKey())
}

func (p defaultSidebarSelectionIntentPolicy) rangeAnchorPolicyOrDefault() SidebarSelectionRangeAnchorPolicy {
	if p.rangeAnchorPolicy == nil {
		return singleSelectedSidebarRangeAnchorPolicy{}
	}
	return p.rangeAnchorPolicy
}

func (p defaultSidebarSelectionIntentPolicy) WithRangeAnchorPolicy(policy SidebarSelectionRangeAnchorPolicy) SidebarSelectionIntentPolicy {
	p.rangeAnchorPolicy = policy
	return p
}

func newDefaultSidebarSelectionIntentPolicy(anchor SidebarSelectionRangeAnchorPolicy) SidebarSelectionIntentPolicy {
	return defaultSidebarSelectionIntentPolicy{
		rangeAnchorPolicy: anchor,
	}
}

func (p defaultSidebarSelectionIntentPolicy) ResolveIntent(sidebar SidebarSelectionReader, clickedKey string, mouse tea.Mouse) sidebarSelectionIntent {
	clickedKey = strings.TrimSpace(clickedKey)
	if sidebar == nil || clickedKey == "" {
		return sidebarSelectionIntent{kind: sidebarSelectionIntentNone}
	}
	if mouse.Mod.Contains(tea.ModShift) {
		anchor := p.rangeAnchorPolicyOrDefault().ResolveAnchor(sidebar, clickedKey, mouse)
		if anchor != "" {
			return sidebarSelectionIntent{
				kind:      sidebarSelectionIntentRangeAdd,
				targetKey: clickedKey,
				anchorKey: anchor,
			}
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
		return sidebar.AddSelectionRangeByKeys(anchor, target)
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
			m.sidebarSelectionIntentPolicy = newDefaultSidebarSelectionIntentPolicy(m.sidebarSelectionRangeAnchorPolicyOrDefault())
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

func WithSidebarSelectionRangeAnchorPolicy(policy SidebarSelectionRangeAnchorPolicy) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if policy == nil {
			m.sidebarSelectionRangeAnchorPolicy = singleSelectedSidebarRangeAnchorPolicy{}
		} else {
			m.sidebarSelectionRangeAnchorPolicy = policy
		}
		if aware, ok := m.sidebarSelectionIntentPolicy.(sidebarSelectionRangeAnchorAware); ok {
			m.sidebarSelectionIntentPolicy = aware.WithRangeAnchorPolicy(m.sidebarSelectionRangeAnchorPolicyOrDefault())
		}
	}
}

func (m *Model) sidebarSelectionIntentPolicyOrDefault() SidebarSelectionIntentPolicy {
	anchorPolicy := SidebarSelectionRangeAnchorPolicy(singleSelectedSidebarRangeAnchorPolicy{})
	if m != nil {
		anchorPolicy = m.sidebarSelectionRangeAnchorPolicyOrDefault()
	}
	if m == nil || m.sidebarSelectionIntentPolicy == nil {
		return newDefaultSidebarSelectionIntentPolicy(anchorPolicy)
	}
	if aware, ok := m.sidebarSelectionIntentPolicy.(sidebarSelectionRangeAnchorAware); ok {
		return aware.WithRangeAnchorPolicy(m.sidebarSelectionRangeAnchorPolicyOrDefault())
	}
	return m.sidebarSelectionIntentPolicy
}

func (m *Model) sidebarSelectionServiceOrDefault() SidebarSelectionService {
	if m == nil || m.sidebarSelectionService == nil {
		return defaultSidebarSelectionService{}
	}
	return m.sidebarSelectionService
}

func (m *Model) sidebarSelectionRangeAnchorPolicyOrDefault() SidebarSelectionRangeAnchorPolicy {
	if m == nil || m.sidebarSelectionRangeAnchorPolicy == nil {
		return singleSelectedSidebarRangeAnchorPolicy{}
	}
	return m.sidebarSelectionRangeAnchorPolicy
}
