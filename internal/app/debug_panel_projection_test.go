package app

import "testing"

type testDebugProjectionPolicy struct {
	max int
}

func (p testDebugProjectionPolicy) MaxTrackedProjectionTokens() int {
	return p.max
}

type testDebugProjectionPresenter struct{}

func (testDebugProjectionPresenter) Present(entries []DebugStreamEntry, _ int, _ DebugPanelPresentationState) DebugPanelPresentation {
	blocks := make([]ChatBlock, 0, len(entries))
	copyByID := make(map[string]string, len(entries))
	for _, entry := range entries {
		blocks = append(blocks, ChatBlock{ID: entry.ID, Role: ChatRoleSystem, Text: entry.Display})
		copyByID[entry.ID] = entry.Display
	}
	return DebugPanelPresentation{
		Blocks:       blocks,
		CopyTextByID: copyByID,
	}
}

type testDebugProjectionRenderer struct{}

func (testDebugProjectionRenderer) Render(blocks []ChatBlock, _ int, _ map[string]ChatBlockMetaPresentation) (string, []renderedBlockSpan) {
	if len(blocks) == 0 {
		return "", nil
	}
	return blocks[len(blocks)-1].Text, []renderedBlockSpan{{ID: blocks[len(blocks)-1].ID}}
}

func TestDefaultDebugPanelProjectionTrackerTreatsNonPositiveMaxAsOne(t *testing.T) {
	tracker := newDefaultDebugPanelProjectionTracker()
	_ = tracker.Next(0)
	second := tracker.Next(-2)
	if len(tracker.order) != 1 {
		t.Fatalf("expected non-positive max tracked to retain one token, got %d", len(tracker.order))
	}
	if tracker.order[0] != second {
		t.Fatalf("expected latest token to be retained, got %#v", tracker.order)
	}
}

func TestDefaultDebugPanelProjectionTrackerEnforcesMaxTrackedTokens(t *testing.T) {
	tracker := newDefaultDebugPanelProjectionTracker()
	_ = tracker.Next(2)
	second := tracker.Next(2)
	third := tracker.Next(2)

	if len(tracker.order) != 2 {
		t.Fatalf("expected tracker to keep only 2 tokens, got %d", len(tracker.order))
	}
	if tracker.order[0] != second || tracker.order[1] != third {
		t.Fatalf("unexpected retained token order %#v", tracker.order)
	}
	if tracker.IsCurrent(second) {
		t.Fatalf("expected previous token to be stale")
	}
	if !tracker.IsCurrent(third) {
		t.Fatalf("expected latest token to be current")
	}

	tracker.Consume(third)
	if tracker.IsCurrent(third) {
		t.Fatalf("expected consumed token to be removed from current set")
	}
}

func TestDebugPanelProjectionCoordinatorDropsStaleResultsByPolicy(t *testing.T) {
	tracker := newDefaultDebugPanelProjectionTracker()
	coordinator := NewDefaultDebugPanelProjectionCoordinator(testDebugProjectionPolicy{max: 1}, tracker)
	req := DebugPanelProjectionRequest{
		Entries:      []DebugStreamEntry{{ID: "debug-1", Display: "one"}},
		Width:        80,
		ExpandedByID: map[string]bool{},
		Presenter:    testDebugProjectionPresenter{},
		Renderer:     testDebugProjectionRenderer{},
	}
	firstCmd := coordinator.Schedule(req)
	if firstCmd == nil {
		t.Fatalf("expected first projection command")
	}
	secondCmd := coordinator.Schedule(req)
	if secondCmd == nil {
		t.Fatalf("expected second projection command")
	}

	firstMsg, ok := firstCmd().(debugPanelProjectedMsg)
	if !ok {
		t.Fatalf("expected debugPanelProjectedMsg from first cmd")
	}
	secondMsg, ok := secondCmd().(debugPanelProjectedMsg)
	if !ok {
		t.Fatalf("expected debugPanelProjectedMsg from second cmd")
	}
	if coordinator.IsCurrent(firstMsg.projectionSeq) {
		t.Fatalf("expected first projection to be stale after second schedule")
	}
	if !coordinator.IsCurrent(secondMsg.projectionSeq) {
		t.Fatalf("expected second projection to be current")
	}
	if len(tracker.order) != 1 {
		t.Fatalf("expected policy max=1 to keep one tracked token, got %d", len(tracker.order))
	}
	coordinator.Consume(secondMsg.projectionSeq)
	if coordinator.IsCurrent(secondMsg.projectionSeq) {
		t.Fatalf("expected consumed projection to no longer be current")
	}
}

func TestDebugPanelProjectionCoordinatorScheduleUsesDefaultDeps(t *testing.T) {
	coordinator := NewDefaultDebugPanelProjectionCoordinator(nil, nil)
	cmd := coordinator.Schedule(DebugPanelProjectionRequest{
		Entries: []DebugStreamEntry{{ID: "debug-1", Display: "payload"}},
		Width:   0,
	})
	if cmd == nil {
		t.Fatalf("expected projection command when entries are present")
	}
	msg, ok := cmd().(debugPanelProjectedMsg)
	if !ok {
		t.Fatalf("expected debugPanelProjectedMsg, got %T", cmd())
	}
	if msg.projectionSeq <= 0 {
		t.Fatalf("expected positive projection sequence, got %d", msg.projectionSeq)
	}
	if msg.empty {
		t.Fatalf("expected non-empty projection output")
	}
}

func TestDebugPanelProjectionCoordinatorNilReceiverSafety(t *testing.T) {
	var coordinator *defaultDebugPanelProjectionCoordinator
	if cmd := coordinator.Schedule(DebugPanelProjectionRequest{}); cmd != nil {
		t.Fatalf("expected nil receiver schedule to return nil command")
	}
	if !coordinator.IsCurrent(0) {
		t.Fatalf("expected nil receiver to treat non-positive seq as current")
	}
	coordinator.Consume(1)
	coordinator.Invalidate()
}
