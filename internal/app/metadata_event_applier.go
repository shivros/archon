package app

import (
	"strings"
	"time"

	"control/internal/guidedworkflows"
	"control/internal/types"
)

type MetadataEventApplyResult struct {
	SidebarDirty        bool
	GuidedWorkflowDirty bool
}

type MetadataEventApplier interface {
	Apply(m *Model, event types.MetadataEvent) MetadataEventApplyResult
}

type defaultMetadataEventApplier struct{}

func (defaultMetadataEventApplier) Apply(m *Model, event types.MetadataEvent) MetadataEventApplyResult {
	if m == nil {
		return MetadataEventApplyResult{}
	}
	switch strings.TrimSpace(event.Type) {
	case types.MetadataEventTypeSessionUpdated:
		if applySessionMetadataEventPatch(m, event.Session) {
			return MetadataEventApplyResult{SidebarDirty: true}
		}
	case types.MetadataEventTypeWorkflowRunUpdated:
		if applyWorkflowMetadataEventPatch(m, event.Workflow) {
			return MetadataEventApplyResult{SidebarDirty: true, GuidedWorkflowDirty: true}
		}
	}
	return MetadataEventApplyResult{}
}

func applySessionMetadataEventPatch(m *Model, event *types.MetadataEntityUpdated) bool {
	if m == nil || event == nil {
		return false
	}
	sessionID := strings.TrimSpace(event.ID)
	if sessionID == "" {
		return false
	}
	title := strings.TrimSpace(event.Title)
	changed := false
	for _, session := range m.sessions {
		if session == nil || strings.TrimSpace(session.ID) != sessionID {
			continue
		}
		if strings.TrimSpace(session.Title) != title {
			session.Title = title
			changed = true
		}
	}
	if m.sessionMeta == nil {
		m.sessionMeta = map[string]*types.SessionMeta{}
	}
	meta := m.sessionMeta[sessionID]
	if meta == nil {
		meta = &types.SessionMeta{SessionID: sessionID}
		m.sessionMeta[sessionID] = meta
	}
	if strings.TrimSpace(meta.Title) != title {
		meta.Title = title
		changed = true
	}
	if !event.UpdatedAt.IsZero() {
		ts := event.UpdatedAt.UTC()
		meta.LastActiveAt = &ts
	}
	return changed
}

func applyWorkflowMetadataEventPatch(m *Model, event *types.MetadataEntityUpdated) bool {
	if m == nil || event == nil {
		return false
	}
	runID := strings.TrimSpace(event.ID)
	if runID == "" {
		return false
	}
	title := strings.TrimSpace(event.Title)
	updatedAt := event.UpdatedAt.UTC()
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	run := &guidedworkflows.WorkflowRun{
		ID:           runID,
		TemplateName: title,
		CreatedAt:    updatedAt,
	}
	before := ""
	for _, existing := range m.workflowRuns {
		if existing == nil || strings.TrimSpace(existing.ID) != runID {
			continue
		}
		before = strings.TrimSpace(existing.TemplateName)
		break
	}
	m.upsertWorkflowRun(run)
	return before != title
}
