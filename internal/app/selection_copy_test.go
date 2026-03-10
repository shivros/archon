package app

import (
	"context"
	"reflect"
	"testing"

	"control/internal/guidedworkflows"
	"control/internal/types"
)

type staticSelectionCopyValueResolver struct {
	value string
	ok    bool
}

func (r staticSelectionCopyValueResolver) Resolve(*sidebarItem) (string, bool) {
	return r.value, r.ok
}

type recordingSelectionCopyPayloadBuilder struct {
	payload    string
	copied     int
	skipped    int
	calls      int
	lastItems  []*sidebarItem
	returnOnce bool
}

func (b *recordingSelectionCopyPayloadBuilder) Build(items []*sidebarItem) (string, int, int) {
	b.calls++
	b.lastItems = append([]*sidebarItem(nil), items...)
	return b.payload, b.copied, b.skipped
}

type testClipboardService struct {
	text  string
	calls int
	err   error
}

func (s *testClipboardService) Copy(_ context.Context, text string) (clipboardMethod, error) {
	s.calls++
	s.text = text
	return clipboardMethodSystem, s.err
}

func TestDefaultSelectionCopyPayloadBuilderBuildMixedSelection(t *testing.T) {
	builder := NewDefaultSelectionCopyPayloadBuilder()
	items := []*sidebarItem{
		{
			kind:      sidebarWorkspace,
			workspace: &types.Workspace{ID: "ws1", RepoPath: "/tmp/ws1"},
		},
		{
			kind:       sidebarWorkflow,
			workflowID: "gwf-1",
			workflow:   &guidedworkflows.WorkflowRun{ID: "gwf-1"},
		},
		{
			kind:    sidebarSession,
			session: &types.Session{ID: "s1"},
		},
		{
			kind:    sidebarSession,
			session: &types.Session{ID: "s2"},
		},
	}

	payload, copiedCount, skippedCount := builder.Build(items)
	const want = "/tmp/ws1\ngwf-1\ns1\ns2"
	if payload != want {
		t.Fatalf("unexpected payload\nwant:\n%s\n\ngot:\n%s", want, payload)
	}
	if copiedCount != 4 {
		t.Fatalf("expected copied count 4, got %d", copiedCount)
	}
	if skippedCount != 0 {
		t.Fatalf("expected skipped count 0, got %d", skippedCount)
	}
}

func TestDefaultSelectionCopyPayloadBuilderSkipsUnsupportedOrEmpty(t *testing.T) {
	builder := NewDefaultSelectionCopyPayloadBuilder()
	items := []*sidebarItem{
		{
			kind:      sidebarWorkspace,
			workspace: &types.Workspace{ID: "ws1", RepoPath: ""},
		},
		{
			kind:     sidebarWorktree,
			worktree: &types.Worktree{ID: "wt1", WorkspaceID: "ws1", Path: "/tmp/ws1/wt1"},
		},
		{
			kind: sidebarRecentsAll,
		},
		{
			kind:       sidebarWorkflow,
			workflowID: "gwf-1",
			workflow:   &guidedworkflows.WorkflowRun{ID: "gwf-1"},
		},
		{
			kind:    sidebarSession,
			session: &types.Session{ID: " "},
		},
	}

	payload, copiedCount, skippedCount := builder.Build(items)
	if payload != "gwf-1" {
		t.Fatalf("expected only workflow id payload, got %q", payload)
	}
	if copiedCount != 1 {
		t.Fatalf("expected copied count 1, got %d", copiedCount)
	}
	if skippedCount != 4 {
		t.Fatalf("expected skipped count 4, got %d", skippedCount)
	}
}

func TestDefaultSelectionCopyPayloadBuilderDedupesBySidebarKey(t *testing.T) {
	builder := NewDefaultSelectionCopyPayloadBuilder()
	items := []*sidebarItem{
		{
			kind:    sidebarSession,
			session: &types.Session{ID: "s1"},
		},
		{
			kind:    sidebarSession,
			session: &types.Session{ID: "s1"},
		},
		{
			kind:      sidebarWorkspace,
			workspace: &types.Workspace{ID: "ws1", RepoPath: "/tmp/ws1"},
		},
		{
			kind:      sidebarWorkspace,
			workspace: &types.Workspace{ID: "ws1", RepoPath: "/tmp/ws1"},
		},
	}

	payload, copiedCount, skippedCount := builder.Build(items)
	const want = "s1\n/tmp/ws1"
	if payload != want {
		t.Fatalf("unexpected deduped payload\nwant:\n%s\n\ngot:\n%s", want, payload)
	}
	if copiedCount != 2 {
		t.Fatalf("expected copied count 2, got %d", copiedCount)
	}
	if skippedCount != 0 {
		t.Fatalf("expected skipped count 0, got %d", skippedCount)
	}
}

func TestWithSelectionCopyPayloadBuilderNilModelNoop(t *testing.T) {
	var nilModel *Model
	WithSelectionCopyPayloadBuilder(NewDefaultSelectionCopyPayloadBuilder())(nilModel)
}

func TestWithSelectionCopyPayloadBuilderNilUsesDefault(t *testing.T) {
	m := NewModel(nil, WithSelectionCopyPayloadBuilder(nil))
	if m.selectionCopyPayloadBuilder == nil {
		t.Fatalf("expected default selection copy payload builder")
	}
}

func TestDedupeSidebarItemsByKeySkipsNilAndEmptyKeys(t *testing.T) {
	items := []*sidebarItem{
		nil,
		{kind: sidebarItemKind(999)}, // empty key
		{kind: sidebarSession, session: &types.Session{ID: "s1"}},
		{kind: sidebarSession, session: &types.Session{ID: "s1"}},
		{kind: sidebarWorkflow, workflowID: "gwf-1"},
		{kind: sidebarWorkflow, workflowID: "gwf-1"},
	}
	got := dedupeSidebarItemsByKey(items)
	if len(got) != 2 {
		t.Fatalf("expected 2 deduped items, got %d", len(got))
	}
	if got[0].key() != "sess:s1" || got[1].key() != "gwf:gwf-1" {
		t.Fatalf("unexpected deduped keys: %q, %q", got[0].key(), got[1].key())
	}
}

func TestResolveSelectionCopyValueRejectsEmptyResolverValue(t *testing.T) {
	item := &sidebarItem{kind: sidebarSession, session: &types.Session{ID: "s1"}}
	value, ok := resolveSelectionCopyValue([]SelectionCopyValueResolver{
		staticSelectionCopyValueResolver{value: "   ", ok: true},
		staticSelectionCopyValueResolver{value: "s1", ok: true},
	}, item)
	if ok {
		t.Fatalf("expected empty resolver value to be rejected")
	}
	if value != "" {
		t.Fatalf("expected empty resolved value, got %q", value)
	}
}

func TestResolveSelectionCopyValueSkipsNilResolver(t *testing.T) {
	item := &sidebarItem{kind: sidebarSession, session: &types.Session{ID: "s1"}}
	value, ok := resolveSelectionCopyValue([]SelectionCopyValueResolver{
		nil,
		staticSelectionCopyValueResolver{value: "s1", ok: true},
	}, item)
	if !ok || value != "s1" {
		t.Fatalf("expected resolver chain to skip nil resolver, got ok=%v value=%q", ok, value)
	}
}

func TestCopySidebarSelectionIDsCmdUsesInjectedPayloadBuilder(t *testing.T) {
	builder := &recordingSelectionCopyPayloadBuilder{
		payload: "/custom/ws\ngwf-custom\nsess-custom",
		copied:  3,
	}
	clipboard := &testClipboardService{}
	m := NewModel(
		nil,
		WithSelectionCopyPayloadBuilder(builder),
		WithClipboardService(clipboard),
	)
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.sidebar.Apply(m.workspaces, map[string][]*types.Worktree{}, nil, nil, nil, "", "", false)

	cmd := m.copySidebarSelectionIDsCmd()
	if cmd == nil {
		t.Fatalf("expected clipboard copy command")
	}
	msg := cmd()
	result, ok := msg.(clipboardResultMsg)
	if !ok {
		t.Fatalf("expected clipboardResultMsg, got %T", msg)
	}
	if result.err != nil {
		t.Fatalf("unexpected copy error: %v", result.err)
	}
	if result.success != "copied 3 id(s)" {
		t.Fatalf("unexpected success text %q", result.success)
	}
	if clipboard.text != builder.payload {
		t.Fatalf("expected injected payload %q, got %q", builder.payload, clipboard.text)
	}
	if builder.calls != 1 {
		t.Fatalf("expected payload builder call count 1, got %d", builder.calls)
	}
	wantItems := m.selectedItemsOrFocused()
	if !reflect.DeepEqual(builder.lastItems, wantItems) {
		t.Fatalf("expected injected builder to receive selected items")
	}
}
