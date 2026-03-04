package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

type stubFileLinkResolver struct{}

func (stubFileLinkResolver) Resolve(rawTarget string) (ResolvedFileLink, error) {
	return ResolvedFileLink{RawTarget: rawTarget, Path: strings.TrimSpace(rawTarget)}, nil
}

type observedFileLinkOpener struct {
	opened []ResolvedFileLink
	err    error
}

func (o *observedFileLinkOpener) Open(_ context.Context, target ResolvedFileLink) error {
	o.opened = append(o.opened, target)
	return o.err
}

func TestMouseReducerTranscriptLinkClickOpensFile(t *testing.T) {
	opener := &observedFileLinkOpener{}
	m := NewModel(nil, WithFileLinkResolver(stubFileLinkResolver{}), WithFileLinkOpener(opener))
	m.setSnapshotBlocks([]ChatBlock{{ID: "b1", Role: ChatRoleAgent, Text: "see [main](/tmp/main.go)"}})
	m.resize(120, 40)
	layout := m.resolveMouseLayout()

	if len(m.contentBlockSpans) != 1 || len(m.contentBlockSpans[0].LinkHits) == 0 {
		t.Fatalf("expected transcript link hit metadata, got %#v", m.contentBlockSpans)
	}
	hit := m.contentBlockSpans[0].LinkHits[0]
	x := layout.rightStart + hit.Start
	y := hit.Line - m.viewport.YOffset() + 1

	handled := m.handleMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y})
	if !handled {
		t.Fatalf("expected transcript link click to be handled")
	}
	flushPendingMouseCmd(t, &m)
	if len(opener.opened) != 1 || opener.opened[0].Path != "/tmp/main.go" {
		t.Fatalf("expected opened transcript link target, got %#v", opener.opened)
	}
}

func TestMouseReducerNotesPanelLinkClickOpensFile(t *testing.T) {
	opener := &observedFileLinkOpener{}
	m := NewModel(nil, WithFileLinkResolver(stubFileLinkResolver{}), WithFileLinkOpener(opener))
	m.notesPanelOpen = true
	m.resize(120, 40)
	m.notesPanelBlocks = []ChatBlock{{ID: "n1", Role: ChatRoleSessionNote, Text: "open [note](/tmp/note.md)"}}
	m.renderNotesPanel()
	layout := m.resolveMouseLayout()

	if len(m.notesPanelSpans) != 1 || len(m.notesPanelSpans[0].LinkHits) == 0 {
		t.Fatalf("expected notes panel link hit metadata, got %#v", m.notesPanelSpans)
	}
	hit := m.notesPanelSpans[0].LinkHits[0]
	x := layout.panelStart + hit.Start
	y := hit.Line - m.notesPanelViewport.YOffset() + 1

	handled := m.reduceNotesPanelLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y}, layout)
	if !handled {
		t.Fatalf("expected notes panel link click to be handled")
	}
	flushPendingMouseCmd(t, &m)
	if len(opener.opened) != 1 || opener.opened[0].Path != "/tmp/note.md" {
		t.Fatalf("expected opened notes panel link target, got %#v", opener.opened)
	}
}

func TestOpenFileLinkCmdResolverErrorSetsWarning(t *testing.T) {
	m := NewModel(nil, WithFileLinkResolver(errorFileLinkResolver{err: errors.New("bad link")}))
	cmd := m.openFileLinkCmd("not-a-file-link")
	if cmd != nil {
		t.Fatalf("expected nil command on resolver failure")
	}
	if !strings.Contains(strings.ToLower(m.status), "unsupported link") {
		t.Fatalf("expected unsupported link warning, got %q", m.status)
	}
}

func TestOpenFileLinkCmdOpenerErrorPublishesFailureMsg(t *testing.T) {
	m := NewModel(nil, WithFileLinkResolver(stubFileLinkResolver{}), WithFileLinkOpener(&observedFileLinkOpener{err: errors.New("open boom")}))
	cmd := m.openFileLinkCmd("/tmp/main.go")
	if cmd == nil {
		t.Fatalf("expected open command")
	}
	msg := cmd()
	result, ok := msg.(fileLinkOpenResultMsg)
	if !ok {
		t.Fatalf("expected fileLinkOpenResultMsg, got %T", msg)
	}
	if result.err == nil || !strings.Contains(result.err.Error(), "open boom") {
		t.Fatalf("expected opener error in result, got %#v", result)
	}
}

func TestOpenTranscriptFileLinkByViewportPositionMissReturnsFalse(t *testing.T) {
	m := NewModel(nil, WithFileLinkResolver(stubFileLinkResolver{}), WithFileLinkOpener(&observedFileLinkOpener{}))
	m.setSnapshotBlocks([]ChatBlock{{ID: "b1", Role: ChatRoleAgent, Text: "see [main](/tmp/main.go)"}})
	m.resize(120, 40)
	handled, cmd := m.openTranscriptFileLinkByViewportPosition(0, 0)
	if handled {
		t.Fatalf("expected miss at unrelated coordinates")
	}
	if cmd != nil {
		t.Fatalf("expected no command on miss")
	}
}

func TestOpenNotesPanelFileLinkByViewportPositionMissReturnsFalse(t *testing.T) {
	m := NewModel(nil, WithFileLinkResolver(stubFileLinkResolver{}), WithFileLinkOpener(&observedFileLinkOpener{}))
	m.notesPanelOpen = true
	m.resize(120, 40)
	m.notesPanelBlocks = []ChatBlock{{ID: "n1", Role: ChatRoleSessionNote, Text: "open [note](/tmp/note.md)"}}
	m.renderNotesPanel()
	handled, cmd := m.openNotesPanelFileLinkByViewportPosition(0, 0)
	if handled {
		t.Fatalf("expected miss at unrelated panel coordinates")
	}
	if cmd != nil {
		t.Fatalf("expected no command on miss")
	}
}

func TestReduceTranscriptLinkLeftPressMouseGuards(t *testing.T) {
	m := NewModel(nil, WithFileLinkResolver(stubFileLinkResolver{}), WithFileLinkOpener(&observedFileLinkOpener{}))
	m.setSnapshotBlocks([]ChatBlock{{ID: "b1", Role: ChatRoleAgent, Text: "see [main](/tmp/main.go)"}})
	m.resize(120, 40)
	layout := m.resolveMouseLayout()

	if m.reduceTranscriptLinkLeftPressMouse(tea.MouseReleaseMsg{Button: tea.MouseLeft, X: layout.rightStart, Y: 1}, layout) {
		t.Fatalf("expected non-click message to be ignored")
	}
	if m.reduceTranscriptLinkLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: 0, Y: 1}, layout) {
		t.Fatalf("expected click outside transcript pane to be ignored")
	}
}

type errorFileLinkResolver struct {
	err error
}

func (r errorFileLinkResolver) Resolve(string) (ResolvedFileLink, error) {
	return ResolvedFileLink{}, r.err
}
