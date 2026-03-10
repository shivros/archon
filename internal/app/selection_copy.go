package app

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

type SelectionCopyValueResolver interface {
	Resolve(*sidebarItem) (string, bool)
}

type SelectionCopyPayloadBuilder interface {
	Build(items []*sidebarItem) (payload string, copiedCount int, skippedCount int)
}

type defaultSelectionCopyPayloadBuilder struct {
	resolvers []SelectionCopyValueResolver
}

type workspaceSelectionCopyValueResolver struct{}
type workflowSelectionCopyValueResolver struct{}
type sessionSelectionCopyValueResolver struct{}

func NewDefaultSelectionCopyPayloadBuilder() SelectionCopyPayloadBuilder {
	return defaultSelectionCopyPayloadBuilder{
		resolvers: []SelectionCopyValueResolver{
			workspaceSelectionCopyValueResolver{},
			workflowSelectionCopyValueResolver{},
			sessionSelectionCopyValueResolver{},
		},
	}
}

func (r workspaceSelectionCopyValueResolver) Resolve(item *sidebarItem) (string, bool) {
	if item == nil || item.kind != sidebarWorkspace || item.workspace == nil {
		return "", false
	}
	repoPath := strings.TrimSpace(item.workspace.RepoPath)
	if repoPath == "" {
		return "", false
	}
	return repoPath, true
}

func (r workflowSelectionCopyValueResolver) Resolve(item *sidebarItem) (string, bool) {
	if item == nil || item.kind != sidebarWorkflow {
		return "", false
	}
	runID := strings.TrimSpace(item.workflowRunID())
	if runID == "" {
		return "", false
	}
	return runID, true
}

func (r sessionSelectionCopyValueResolver) Resolve(item *sidebarItem) (string, bool) {
	if item == nil || item.kind != sidebarSession || item.session == nil {
		return "", false
	}
	sessionID := strings.TrimSpace(item.session.ID)
	if sessionID == "" {
		return "", false
	}
	return sessionID, true
}

func (b defaultSelectionCopyPayloadBuilder) Build(items []*sidebarItem) (string, int, int) {
	items = dedupeSidebarItemsByKey(items)
	if len(items) == 0 {
		return "", 0, 0
	}
	values := make([]string, 0, len(items))
	skippedCount := 0
	for _, item := range items {
		value, ok := resolveSelectionCopyValue(b.resolvers, item)
		if !ok {
			skippedCount++
			continue
		}
		values = append(values, value)
	}
	if len(values) == 0 {
		return "", 0, skippedCount
	}
	return strings.Join(values, "\n"), len(values), skippedCount
}

func dedupeSidebarItemsByKey(items []*sidebarItem) []*sidebarItem {
	if len(items) <= 1 {
		return items
	}
	out := make([]*sidebarItem, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		if item == nil {
			continue
		}
		key := strings.TrimSpace(item.key())
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}

func resolveSelectionCopyValue(resolvers []SelectionCopyValueResolver, item *sidebarItem) (string, bool) {
	for _, resolver := range resolvers {
		if resolver == nil {
			continue
		}
		value, ok := resolver.Resolve(item)
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		if value == "" {
			return "", false
		}
		return value, true
	}
	return "", false
}

func (m *Model) selectionCopyPayloadBuilderOrDefault() SelectionCopyPayloadBuilder {
	if m == nil || m.selectionCopyPayloadBuilder == nil {
		return NewDefaultSelectionCopyPayloadBuilder()
	}
	return m.selectionCopyPayloadBuilder
}

func WithSelectionCopyPayloadBuilder(builder SelectionCopyPayloadBuilder) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if builder == nil {
			m.selectionCopyPayloadBuilder = NewDefaultSelectionCopyPayloadBuilder()
			return
		}
		m.selectionCopyPayloadBuilder = builder
	}
}

func (m *Model) copySidebarSelectionIDsCmd() tea.Cmd {
	if m == nil {
		return nil
	}
	payload, copiedCount, skippedCount := m.selectionCopyPayloadBuilderOrDefault().Build(m.selectedItemsOrFocused())
	if copiedCount == 0 || strings.TrimSpace(payload) == "" {
		m.setCopyStatusWarning("no workspace/workflow/session selected")
		return nil
	}
	success := fmt.Sprintf("copied %d id(s)", copiedCount)
	if skippedCount > 0 {
		success = fmt.Sprintf("%s, skipped %d", success, skippedCount)
	}
	return m.copyWithStatusCmd(payload, success)
}
