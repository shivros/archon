package app

import tea "charm.land/bubbletea/v2"

type debugPanelProjectionCoordinator interface {
	Schedule(req DebugPanelProjectionRequest) tea.Cmd
	IsCurrent(seq int) bool
	Consume(seq int)
	Invalidate()
}

type DebugPanelProjectionRequest struct {
	Entries      []DebugStreamEntry
	Width        int
	ExpandedByID map[string]bool
	Presenter    debugPanelPresenter
	Renderer     debugPanelBlocksRenderer
}

type debugPanelProjectionTracker interface {
	Next(maxTracked int) int
	IsCurrent(seq int) bool
	Consume(seq int)
	Invalidate(maxTracked int)
}

type defaultDebugPanelProjectionCoordinator struct {
	policy  DebugPanelProjectionPolicy
	tracker debugPanelProjectionTracker
}

func NewDefaultDebugPanelProjectionCoordinator(
	policy DebugPanelProjectionPolicy,
	tracker debugPanelProjectionTracker,
) debugPanelProjectionCoordinator {
	if policy == nil {
		policy = defaultDebugPanelProjectionPolicy{}
	}
	if tracker == nil {
		tracker = newDefaultDebugPanelProjectionTracker()
	}
	return &defaultDebugPanelProjectionCoordinator{
		policy:  policy,
		tracker: tracker,
	}
}

func (c *defaultDebugPanelProjectionCoordinator) Schedule(req DebugPanelProjectionRequest) tea.Cmd {
	if c == nil {
		return nil
	}
	if len(req.Entries) == 0 {
		return nil
	}
	presenter := req.Presenter
	if presenter == nil {
		presenter = NewDefaultDebugPanelPresenter(DefaultDebugPanelDisplayPolicy())
	}
	renderer := req.Renderer
	if renderer == nil {
		renderer = NewDefaultDebugPanelBlocksRenderer()
	}
	width := req.Width
	if width <= 0 {
		width = 1
	}
	seq := c.tracker.Next(c.policy.MaxTrackedProjectionTokens())
	return projectDebugPanelCmd(
		req.Entries,
		width,
		req.ExpandedByID,
		presenter,
		renderer,
		seq,
	)
}

func (c *defaultDebugPanelProjectionCoordinator) IsCurrent(seq int) bool {
	if c == nil || c.tracker == nil {
		return seq <= 0
	}
	return c.tracker.IsCurrent(seq)
}

func (c *defaultDebugPanelProjectionCoordinator) Consume(seq int) {
	if c == nil || c.tracker == nil {
		return
	}
	c.tracker.Consume(seq)
}

func (c *defaultDebugPanelProjectionCoordinator) Invalidate() {
	if c == nil || c.tracker == nil {
		return
	}
	c.tracker.Invalidate(c.policy.MaxTrackedProjectionTokens())
}

type defaultDebugPanelProjectionTracker struct {
	nextSeq int
	latest  int
	tracked map[int]struct{}
	order   []int
}

func newDefaultDebugPanelProjectionTracker() *defaultDebugPanelProjectionTracker {
	return &defaultDebugPanelProjectionTracker{
		tracked: map[int]struct{}{},
	}
}

func (t *defaultDebugPanelProjectionTracker) Next(maxTracked int) int {
	if t == nil {
		return 0
	}
	t.nextSeq++
	seq := t.nextSeq
	t.latest = seq
	t.track(seq, maxTracked)
	return seq
}

func (t *defaultDebugPanelProjectionTracker) IsCurrent(seq int) bool {
	if t == nil {
		return seq <= 0
	}
	if seq <= 0 || t.latest != seq {
		return false
	}
	_, ok := t.tracked[seq]
	return ok
}

func (t *defaultDebugPanelProjectionTracker) Consume(seq int) {
	if t == nil || seq <= 0 {
		return
	}
	t.untrack(seq)
	if t.latest == seq {
		t.latest = 0
	}
}

func (t *defaultDebugPanelProjectionTracker) Invalidate(maxTracked int) {
	if t == nil {
		return
	}
	t.nextSeq++
	t.latest = t.nextSeq
	t.track(t.latest, maxTracked)
}

func (t *defaultDebugPanelProjectionTracker) track(seq int, maxTracked int) {
	if t == nil || seq <= 0 {
		return
	}
	if t.tracked == nil {
		t.tracked = map[int]struct{}{}
	}
	if _, exists := t.tracked[seq]; !exists {
		t.tracked[seq] = struct{}{}
		t.order = append(t.order, seq)
	}
	if maxTracked <= 0 {
		maxTracked = 1
	}
	for len(t.order) > maxTracked {
		evict := t.order[0]
		t.order = t.order[1:]
		delete(t.tracked, evict)
	}
}

func (t *defaultDebugPanelProjectionTracker) untrack(seq int) {
	if t == nil || len(t.order) == 0 {
		return
	}
	delete(t.tracked, seq)
	next := make([]int, 0, len(t.order))
	for _, current := range t.order {
		if current == seq {
			continue
		}
		next = append(next, current)
	}
	t.order = next
}
