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
	target := strings.TrimSpace(rawTarget)
	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
		return ResolvedFileLink{RawTarget: rawTarget, Kind: FileLinkTargetKindURL, URL: target}, nil
	}
	return ResolvedFileLink{RawTarget: rawTarget, Kind: FileLinkTargetKindFile, FilePath: target}, nil
}

type observedFileLinkOpener struct {
	opened []ResolvedFileLink
	err    error
}

func (o *observedFileLinkOpener) Open(_ context.Context, target ResolvedFileLink) error {
	o.opened = append(o.opened, target)
	return o.err
}

func TestMouseReducerTranscriptCtrlClickLinkOpensFile(t *testing.T) {
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

	handled := m.handleMouse(tea.MouseClickMsg{Button: tea.MouseLeft, Mod: tea.ModCtrl, X: x, Y: y})
	if !handled {
		t.Fatalf("expected transcript ctrl+click link click to be handled")
	}
	flushPendingMouseCmd(t, &m)
	if len(opener.opened) != 1 || opener.opened[0].FilePath != "/tmp/main.go" {
		t.Fatalf("expected opened transcript link target, got %#v", opener.opened)
	}
}

func TestMouseReducerTranscriptPlainClickLinkDoesNotOpenFile(t *testing.T) {
	opener := &observedFileLinkOpener{}
	m := NewModel(nil, WithFileLinkResolver(stubFileLinkResolver{}), WithFileLinkOpener(opener))
	m.setSnapshotBlocks([]ChatBlock{{ID: "b1", Role: ChatRoleAgent, Text: "see [main](/tmp/main.go)"}})
	m.resize(120, 40)
	layout := m.resolveMouseLayout()

	hit := m.contentBlockSpans[0].LinkHits[0]
	x := layout.rightStart + hit.Start
	y := hit.Line - m.viewport.YOffset() + 1
	if !m.handleMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y}) {
		t.Fatalf("expected plain link click to be handled by non-link reducers")
	}
	flushPendingMouseCmd(t, &m)
	if len(opener.opened) != 0 {
		t.Fatalf("expected plain click not to open link target, got %#v", opener.opened)
	}
}

func TestMouseReducerNotesPanelCtrlClickLinkOpensFile(t *testing.T) {
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

	handled := m.reduceNotesPanelLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, Mod: tea.ModCtrl, X: x, Y: y}, layout)
	if !handled {
		t.Fatalf("expected notes panel ctrl+click link click to be handled")
	}
	flushPendingMouseCmd(t, &m)
	if len(opener.opened) != 1 || opener.opened[0].FilePath != "/tmp/note.md" {
		t.Fatalf("expected opened notes panel link target, got %#v", opener.opened)
	}
}

func TestMouseReducerTranscriptCtrlClickURLLinkOpensURL(t *testing.T) {
	opener := &observedFileLinkOpener{}
	m := NewModel(nil, WithFileLinkResolver(stubFileLinkResolver{}), WithFileLinkOpener(opener))
	m.setSnapshotBlocks([]ChatBlock{{ID: "b1", Role: ChatRoleAgent, Text: "see [site](https://example.com/docs)"}})
	m.resize(120, 40)
	layout := m.resolveMouseLayout()

	if len(m.contentBlockSpans) != 1 || len(m.contentBlockSpans[0].LinkHits) == 0 {
		t.Fatalf("expected transcript link hit metadata, got %#v", m.contentBlockSpans)
	}
	hit := m.contentBlockSpans[0].LinkHits[0]
	x := layout.rightStart + hit.Start
	y := hit.Line - m.viewport.YOffset() + 1

	handled := m.handleMouse(tea.MouseClickMsg{Button: tea.MouseLeft, Mod: tea.ModCtrl, X: x, Y: y})
	if !handled {
		t.Fatalf("expected transcript ctrl+click URL link to be handled")
	}
	flushPendingMouseCmd(t, &m)
	if len(opener.opened) != 1 {
		t.Fatalf("expected one URL open request, got %#v", opener.opened)
	}
	if opener.opened[0].Kind != FileLinkTargetKindURL || opener.opened[0].URL != "https://example.com/docs" {
		t.Fatalf("expected URL open target, got %#v", opener.opened[0])
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
	if result.target != "/tmp/main.go" {
		t.Fatalf("expected target path in result, got %#v", result)
	}
}

func TestOpenFileLinkCmdURLPublishesURLTarget(t *testing.T) {
	opener := &observedFileLinkOpener{}
	m := NewModel(nil, WithFileLinkResolver(stubFileLinkResolver{}), WithFileLinkOpener(opener))
	cmd := m.openFileLinkCmd("https://example.com/docs")
	if cmd == nil {
		t.Fatalf("expected open command for URL")
	}
	msg := cmd()
	result, ok := msg.(fileLinkOpenResultMsg)
	if !ok {
		t.Fatalf("expected fileLinkOpenResultMsg, got %T", msg)
	}
	if result.err != nil {
		t.Fatalf("expected URL opener success, got %v", result.err)
	}
	if result.target != "https://example.com/docs" {
		t.Fatalf("expected URL target in result, got %#v", result)
	}
	if len(opener.opened) != 1 || opener.opened[0].URL != "https://example.com/docs" || opener.opened[0].Kind != FileLinkTargetKindURL {
		t.Fatalf("expected URL target to be opened, got %#v", opener.opened)
	}
}

func TestOpenFileLinkCmdFallsBackToDefaultResolverWhenNil(t *testing.T) {
	opener := &observedFileLinkOpener{}
	m := NewModel(nil, WithFileLinkOpener(opener))
	m.fileLinkResolver = nil

	cmd := m.openFileLinkCmd("/tmp/main.go")
	if cmd == nil {
		t.Fatalf("expected fallback resolver to produce command")
	}
	msg := cmd()
	result, ok := msg.(fileLinkOpenResultMsg)
	if !ok {
		t.Fatalf("expected fileLinkOpenResultMsg, got %T", msg)
	}
	if result.err != nil {
		t.Fatalf("expected opener success with fallback resolver, got %v", result.err)
	}
	if len(opener.opened) != 1 || opener.opened[0].Kind != FileLinkTargetKindFile || opener.opened[0].FilePath != "/tmp/main.go" {
		t.Fatalf("expected fallback resolver to open file target, got %#v", opener.opened)
	}
}

func TestOpenFileLinkCmdBuildsCommandWhenOpenerNil(t *testing.T) {
	m := NewModel(nil, WithFileLinkResolver(stubFileLinkResolver{}))
	m.fileLinkOpener = nil
	cmd := m.openFileLinkCmd("/tmp/main.go")
	if cmd == nil {
		t.Fatalf("expected fallback opener path to build command")
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

func TestOpenTranscriptFileLinkByViewportPositionSkipsInvalidHitRange(t *testing.T) {
	m := NewModel(nil, WithFileLinkResolver(stubFileLinkResolver{}), WithFileLinkOpener(&observedFileLinkOpener{}))
	m.contentBlockSpans = []renderedBlockSpan{{
		LinkHits: []renderedLinkHit{{
			Target: "/tmp/main.go",
			Line:   0,
			Start:  5,
			End:    4,
		}},
	}}

	handled, cmd := m.openTranscriptFileLinkByViewportPosition(5, 0)
	if handled {
		t.Fatalf("expected invalid hit range to be ignored")
	}
	if cmd != nil {
		t.Fatalf("expected no command for invalid hit range")
	}
}

func TestOpenNotesPanelFileLinkByViewportPositionSkipsInvalidHitRange(t *testing.T) {
	m := NewModel(nil, WithFileLinkResolver(stubFileLinkResolver{}), WithFileLinkOpener(&observedFileLinkOpener{}))
	m.notesPanelSpans = []renderedBlockSpan{{
		LinkHits: []renderedLinkHit{{
			Target: "/tmp/note.md",
			Line:   0,
			Start:  3,
			End:    2,
		}},
	}}

	handled, cmd := m.openNotesPanelFileLinkByViewportPosition(3, 0)
	if handled {
		t.Fatalf("expected invalid panel hit range to be ignored")
	}
	if cmd != nil {
		t.Fatalf("expected no command for invalid panel hit range")
	}
}

func TestReduceNotesPanelLeftPressMouseCtrlClickNoLinkConsumesWithoutAction(t *testing.T) {
	m := NewModel(nil, WithFileLinkResolver(stubFileLinkResolver{}), WithFileLinkOpener(&observedFileLinkOpener{}))
	m.notesPanelOpen = true
	m.resize(120, 40)
	m.notesPanelBlocks = []ChatBlock{{ID: "n1", Role: ChatRoleSessionNote, Text: "plain note body"}}
	m.renderNotesPanel()
	layout := m.resolveMouseLayout()
	m.status = "ready"

	handled := m.reduceNotesPanelLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, Mod: tea.ModCtrl, X: layout.panelStart + 1, Y: 1}, layout)
	if !handled {
		t.Fatalf("expected ctrl+click without link to be consumed")
	}
	if m.pendingMouseCmd != nil {
		t.Fatalf("expected no action command for ctrl+click without link")
	}
	if m.status != "ready" {
		t.Fatalf("expected status to remain unchanged, got %q", m.status)
	}
}

func TestFileLinkOpenResultMessageStatusFormatting(t *testing.T) {
	m := NewModel(nil)
	handled, cmd := m.reduceStateMessages(fileLinkOpenResultMsg{target: "/tmp/main.go"})
	if !handled {
		t.Fatalf("expected file link result message to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no follow-up command")
	}
	if m.status != "opened /tmp/main.go" {
		t.Fatalf("expected opened-path status, got %q", m.status)
	}

	handled, cmd = m.reduceStateMessages(fileLinkOpenResultMsg{target: ""})
	if !handled {
		t.Fatalf("expected empty-target file link result to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no follow-up command for empty-target result")
	}
	if m.status != "link opened" {
		t.Fatalf("expected generic opened status, got %q", m.status)
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
	if m.reduceTranscriptLinkLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: layout.rightStart, Y: 1}, layout) {
		t.Fatalf("expected non-ctrl click message to be ignored")
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

func TestWithFileLinkResolverOptionNilModelAndNilResolverNoop(t *testing.T) {
	WithFileLinkResolver(stubFileLinkResolver{})(nil)
	m := NewModel(nil)
	original := m.fileLinkResolver
	WithFileLinkResolver(nil)(&m)
	if m.fileLinkResolver != original {
		t.Fatalf("expected nil resolver option to be noop")
	}
}

func TestWithFileLinkOpenerOptionNilModelAndNilOpenerNoop(t *testing.T) {
	WithFileLinkOpener(&observedFileLinkOpener{})(nil)
	m := NewModel(nil)
	original := m.fileLinkOpener
	WithFileLinkOpener(nil)(&m)
	if m.fileLinkOpener != original {
		t.Fatalf("expected nil opener option to be noop")
	}
}
