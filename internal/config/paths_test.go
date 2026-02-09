package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestPaths(t *testing.T) {
	t.Setenv("HOME", filepath.Join(t.TempDir(), "home"))

	dataDir, err := DataDir()
	if err != nil {
		t.Fatalf("DataDir: %v", err)
	}
	if !strings.HasSuffix(dataDir, filepath.Join(".archon")) {
		t.Fatalf("unexpected data dir: %s", dataDir)
	}

	sessionsDir, err := SessionsDir()
	if err != nil {
		t.Fatalf("SessionsDir: %v", err)
	}
	if !strings.HasSuffix(sessionsDir, filepath.Join(".archon", "sessions")) {
		t.Fatalf("unexpected sessions dir: %s", sessionsDir)
	}

	tokenPath, err := TokenPath()
	if err != nil {
		t.Fatalf("TokenPath: %v", err)
	}
	if !strings.HasSuffix(tokenPath, filepath.Join(".archon", "token")) {
		t.Fatalf("unexpected token path: %s", tokenPath)
	}

	workspacesPath, err := WorkspacesPath()
	if err != nil {
		t.Fatalf("WorkspacesPath: %v", err)
	}
	if !strings.HasSuffix(workspacesPath, filepath.Join(".archon", "workspaces.json")) {
		t.Fatalf("unexpected workspaces path: %s", workspacesPath)
	}

	statePath, err := StatePath()
	if err != nil {
		t.Fatalf("StatePath: %v", err)
	}
	if !strings.HasSuffix(statePath, filepath.Join(".archon", "state.json")) {
		t.Fatalf("unexpected state path: %s", statePath)
	}

	keymapPath, err := KeymapPath()
	if err != nil {
		t.Fatalf("KeymapPath: %v", err)
	}
	if !strings.HasSuffix(keymapPath, filepath.Join(".archon", "keymap.json")) {
		t.Fatalf("unexpected keymap path: %s", keymapPath)
	}

	sessionsMetaPath, err := SessionsMetaPath()
	if err != nil {
		t.Fatalf("SessionsMetaPath: %v", err)
	}
	if !strings.HasSuffix(sessionsMetaPath, filepath.Join(".archon", "sessions_meta.json")) {
		t.Fatalf("unexpected sessions meta path: %s", sessionsMetaPath)
	}

	sessionsIndexPath, err := SessionsIndexPath()
	if err != nil {
		t.Fatalf("SessionsIndexPath: %v", err)
	}
	if !strings.HasSuffix(sessionsIndexPath, filepath.Join(".archon", "sessions_index.json")) {
		t.Fatalf("unexpected sessions index path: %s", sessionsIndexPath)
	}

	notesPath, err := NotesPath()
	if err != nil {
		t.Fatalf("NotesPath: %v", err)
	}
	if !strings.HasSuffix(notesPath, filepath.Join(".archon", "notes.json")) {
		t.Fatalf("unexpected notes path: %s", notesPath)
	}
}
