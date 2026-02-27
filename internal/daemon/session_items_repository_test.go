package daemon

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileSessionItemsRepositoryAppendAndReadRoundTrip(t *testing.T) {
	baseDir := t.TempDir()
	repo := &fileSessionItemsRepository{
		baseDirResolver: func() (string, error) { return baseDir, nil },
	}
	sessionID := "sess-roundtrip"
	if err := repo.AppendItems(sessionID, []map[string]any{
		{
			"type": "assistant",
			"message": map[string]any{
				"content": []map[string]any{{"type": "text", "text": "hello"}},
			},
		},
	}); err != nil {
		t.Fatalf("AppendItems: %v", err)
	}
	items, err := repo.ReadItems(sessionID, 50)
	if err != nil {
		t.Fatalf("ReadItems: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one item, got %d", len(items))
	}
	if strings.TrimSpace(asString(items[0]["type"])) != "assistant" {
		t.Fatalf("expected assistant item, got %#v", items[0])
	}
}

func TestFileSessionItemsRepositoryReadMissingFile(t *testing.T) {
	repo := &fileSessionItemsRepository{
		baseDirResolver: func() (string, error) { return t.TempDir(), nil },
	}
	items, err := repo.ReadItems("sess-missing", 20)
	if err != nil {
		t.Fatalf("ReadItems: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected no items for missing file, got %#v", items)
	}
}

func TestFileSessionItemsRepositoryBaseDirResolverError(t *testing.T) {
	repo := &fileSessionItemsRepository{
		baseDirResolver: func() (string, error) { return "", errors.New("resolver failed") },
	}
	if _, err := repo.ReadItems("sess-err", 20); err == nil {
		t.Fatalf("expected read error from resolver")
	}
	if err := repo.AppendItems("sess-err", []map[string]any{{"type": "assistant"}}); err == nil {
		t.Fatalf("expected append error from resolver")
	}
}

func TestNewFileSessionItemsRepositoryUsesManagerBaseDir(t *testing.T) {
	baseDir := t.TempDir()
	manager := &SessionManager{baseDir: baseDir}
	repo, ok := newFileSessionItemsRepository(manager).(*fileSessionItemsRepository)
	if !ok {
		t.Fatalf("expected concrete fileSessionItemsRepository")
	}
	resolved, err := repo.baseDir()
	if err != nil {
		t.Fatalf("baseDir: %v", err)
	}
	if filepath.Clean(resolved) != filepath.Clean(baseDir) {
		t.Fatalf("expected manager base dir %q, got %q", baseDir, resolved)
	}
}

func TestFileSessionItemsRepositoryAppendItemsNoopForEmptyInput(t *testing.T) {
	repo := &fileSessionItemsRepository{
		baseDirResolver: func() (string, error) { return t.TempDir(), nil },
	}
	if err := repo.AppendItems("sess-noop", nil); err != nil {
		t.Fatalf("expected nil error for empty append, got %v", err)
	}
	if err := repo.AppendItems("  ", []map[string]any{{"type": "assistant"}}); err != nil {
		t.Fatalf("expected nil error for empty session id, got %v", err)
	}
}

func TestFileSessionItemsRepositoryAppendBroadcastsPreparedItems(t *testing.T) {
	baseDir := t.TempDir()
	calls := 0
	var gotSession string
	var gotItems []map[string]any
	repo := &fileSessionItemsRepository{
		baseDirResolver: func() (string, error) { return baseDir, nil },
		broadcastItems: func(sessionID string, items []map[string]any) {
			calls++
			gotSession = sessionID
			gotItems = append([]map[string]any(nil), items...)
		},
	}
	err := repo.AppendItems("sess-broadcast", []map[string]any{
		{
			"type": "assistant",
			"message": map[string]any{
				"content": []map[string]any{{"type": "text", "text": "hello"}},
			},
		},
	})
	if err != nil {
		t.Fatalf("AppendItems: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected one broadcast call, got %d", calls)
	}
	if gotSession != "sess-broadcast" {
		t.Fatalf("expected broadcast for sess-broadcast, got %q", gotSession)
	}
	if len(gotItems) != 1 {
		t.Fatalf("expected one broadcast item, got %d", len(gotItems))
	}
	if strings.TrimSpace(asString(gotItems[0]["type"])) != "assistant" {
		t.Fatalf("expected assistant broadcast item, got %#v", gotItems[0])
	}
}
