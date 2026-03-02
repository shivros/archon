package app

import "strings"

type statusHistoryStore struct {
	maxEntries int
	entries    []string
}

func newStatusHistoryStore(maxEntries int) statusHistoryStore {
	if maxEntries <= 0 {
		maxEntries = 1
	}
	return statusHistoryStore{
		maxEntries: maxEntries,
		entries:    make([]string, 0, maxEntries),
	}
}

func (s *statusHistoryStore) Append(message string) {
	if s == nil {
		return
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return
	}
	n := len(s.entries)
	if n > 0 && s.entries[n-1] == message {
		return
	}
	s.entries = append(s.entries, message)
	if len(s.entries) > s.maxEntries {
		overflow := len(s.entries) - s.maxEntries
		s.entries = append([]string{}, s.entries[overflow:]...)
	}
}

func (s *statusHistoryStore) SnapshotNewestFirst() []string {
	if s == nil || len(s.entries) == 0 {
		return nil
	}
	out := make([]string, 0, len(s.entries))
	for i := len(s.entries) - 1; i >= 0; i-- {
		out = append(out, s.entries[i])
	}
	return out
}

type statusHistoryOverlayController struct {
	open          bool
	selectedIndex int
	scrollOffset  int
}

func newStatusHistoryOverlayController() statusHistoryOverlayController {
	return statusHistoryOverlayController{
		selectedIndex: -1,
	}
}

func (c *statusHistoryOverlayController) Open() {
	if c == nil {
		return
	}
	c.open = true
}

func (c *statusHistoryOverlayController) Close() {
	if c == nil {
		return
	}
	c.open = false
	c.selectedIndex = -1
	c.scrollOffset = 0
}

func (c *statusHistoryOverlayController) Toggle() {
	if c == nil {
		return
	}
	if c.open {
		c.Close()
		return
	}
	c.Open()
}

func (c *statusHistoryOverlayController) IsOpen() bool {
	return c != nil && c.open
}

func (c *statusHistoryOverlayController) SelectedIndex() int {
	if c == nil {
		return -1
	}
	return c.selectedIndex
}

func (c *statusHistoryOverlayController) ScrollOffset() int {
	if c == nil {
		return 0
	}
	return c.scrollOffset
}

func (c *statusHistoryOverlayController) Select(index, total, visibleRows int) bool {
	if c == nil || total <= 0 {
		return false
	}
	index = clamp(index, 0, total-1)
	changed := c.selectedIndex != index
	c.selectedIndex = index
	c.ensureVisible(total, visibleRows)
	return changed
}

func (c *statusHistoryOverlayController) Move(delta, total, visibleRows int) bool {
	if c == nil || total <= 0 || delta == 0 {
		return false
	}
	next := c.selectedIndex
	if next < 0 {
		if delta > 0 {
			next = 0
		} else {
			next = total - 1
		}
	} else {
		next += delta
	}
	next = clamp(next, 0, total-1)
	changed := c.selectedIndex != next
	c.selectedIndex = next
	c.ensureVisible(total, visibleRows)
	return changed
}

func (c *statusHistoryOverlayController) Scroll(delta, total, visibleRows int) bool {
	if c == nil || total <= 0 || visibleRows <= 0 || delta == 0 {
		return false
	}
	maxOffset := max(0, total-visibleRows)
	next := clamp(c.scrollOffset+delta, 0, maxOffset)
	if next == c.scrollOffset {
		return false
	}
	c.scrollOffset = next
	return true
}

func (c *statusHistoryOverlayController) Reconcile(total, visibleRows int) {
	if c == nil {
		return
	}
	if total <= 0 {
		c.selectedIndex = -1
		c.scrollOffset = 0
		return
	}
	c.selectedIndex = clamp(c.selectedIndex, -1, total-1)
	maxOffset := max(0, total-visibleRows)
	c.scrollOffset = clamp(c.scrollOffset, 0, maxOffset)
	c.ensureVisible(total, visibleRows)
}

func (c *statusHistoryOverlayController) ensureVisible(total, visibleRows int) {
	if c == nil || total <= 0 || visibleRows <= 0 || c.selectedIndex < 0 {
		return
	}
	maxOffset := max(0, total-visibleRows)
	if c.scrollOffset > maxOffset {
		c.scrollOffset = maxOffset
	}
	if c.selectedIndex < c.scrollOffset {
		c.scrollOffset = c.selectedIndex
	}
	if c.selectedIndex >= c.scrollOffset+visibleRows {
		c.scrollOffset = c.selectedIndex - visibleRows + 1
	}
	c.scrollOffset = clamp(c.scrollOffset, 0, maxOffset)
}
