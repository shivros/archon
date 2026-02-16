package app

import (
	"fmt"
	"testing"
	"time"

	"control/internal/types"
)

const (
	benchWorkspaceID = "ws-bench"
)

func benchmarkModelWithSessions(sessionCount int) *Model {
	m := NewModel(nil)
	m.appState.ActiveWorkspaceGroupIDs = []string{"ungrouped"}
	m.workspaces = []*types.Workspace{{ID: benchWorkspaceID, Name: "Benchmark Workspace"}}
	m.worktrees = map[string][]*types.Worktree{}
	m.sessions = make([]*types.Session, 0, sessionCount)
	m.sessionMeta = make(map[string]*types.SessionMeta, sessionCount)
	now := time.Now().UTC()
	for i := 0; i < sessionCount; i++ {
		id := fmt.Sprintf("s%04d", i)
		m.sessions = append(m.sessions, &types.Session{
			ID:        id,
			Provider:  "codex",
			Status:    types.SessionStatusRunning,
			CreatedAt: now.Add(-time.Duration(i) * time.Second),
			Title:     "Session " + id,
		})
		m.sessionMeta[id] = &types.SessionMeta{SessionID: id, WorkspaceID: benchWorkspaceID}
	}
	m.applySidebarItems()
	m.resize(160, 48)
	if m.sidebar != nil {
		_ = m.sidebar.SelectBySessionID("s0000")
	}
	return &m
}

func benchmarkBlocks(count int) []ChatBlock {
	blocks := make([]ChatBlock, 0, count)
	for i := 0; i < count; i++ {
		role := ChatRoleAgent
		if i%3 == 0 {
			role = ChatRoleUser
		}
		blocks = append(blocks, ChatBlock{
			ID:   fmt.Sprintf("block-%d", i),
			Role: role,
			Text: fmt.Sprintf("## Block %d\n\n- one\n- two\n\nCode:\n```go\nfmt.Println(\"%d\")\n```\n", i, i),
		})
	}
	return blocks
}

func BenchmarkModelActionToggleSessionsSidebar(b *testing.B) {
	m := benchmarkModelWithSessions(40)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.toggleSidebar()
	}
}

func BenchmarkModelActionToggleNotesSidebar(b *testing.B) {
	m := benchmarkModelWithSessions(40)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.toggleNotesPanel()
	}
}

func BenchmarkModelActionExitCompose(b *testing.B) {
	m := benchmarkModelWithSessions(40)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.mode = uiModeCompose
		m.newSession = nil
		m.exitCompose("")
	}
}

func BenchmarkModelActionOpenNewSession(b *testing.B) {
	m := benchmarkModelWithSessions(40)
	if m.sidebar != nil {
		_ = m.sidebar.SelectBySessionID("s0000")
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.mode = uiModeNormal
		m.newSession = nil
		_ = m.enterNewSession()
	}
}

func BenchmarkModelActionSwitchSession(b *testing.B) {
	m := benchmarkModelWithSessions(120)
	targetIDs := []string{"s0001", "s0002", "s0003", "s0004"}
	idx := 0
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		target := targetIDs[idx%len(targetIDs)]
		idx++
		if m.sidebar != nil {
			_ = m.sidebar.SelectBySessionID(target)
		}
		_ = m.onSelectionChangedWithDelay(0)
	}
}

func BenchmarkModelRenderViewportLargeTranscript(b *testing.B) {
	m := benchmarkModelWithSessions(40)
	blocks := benchmarkBlocks(500)
	// Warm up caches so the benchmark captures steady-state interaction cost.
	m.applyBlocks(blocks)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.applyBlocks(blocks)
	}
}

func BenchmarkModelViewLargeTranscript(b *testing.B) {
	m := benchmarkModelWithSessions(40)
	m.applyBlocks(benchmarkBlocks(500))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.View()
	}
}

func BenchmarkModelSessionSwitchPath(b *testing.B) {
	m := benchmarkModelWithSessions(250)
	targetIDs := []string{"s0010", "s0020", "s0030", "s0040", "s0050", "s0060"}
	idx := 0
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		target := targetIDs[idx%len(targetIDs)]
		idx++
		if m.sidebar != nil {
			_ = m.sidebar.SelectBySessionID(target)
		}
		_ = m.onSelectionChangedWithDelay(0)
	}
}
