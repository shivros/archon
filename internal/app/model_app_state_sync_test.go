package app

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"control/internal/types"
)

type appStateSyncStub struct{}

func (a *appStateSyncStub) GetAppState(ctx context.Context) (*types.AppState, error) {
	return nil, nil
}

func (a *appStateSyncStub) UpdateAppState(ctx context.Context, state *types.AppState) (*types.AppState, error) {
	if state == nil {
		return nil, nil
	}
	cloned := *state
	return &cloned, nil
}

func TestSaveAppStateCmdAssignsMonotonicRequestSeq(t *testing.T) {
	m := NewModel(nil)
	m.stateAPI = &appStateSyncStub{}
	m.hasAppState = true
	m.appState.ActiveWorkspaceGroupIDs = []string{"group-1"}

	cmd := m.saveAppStateCmd()
	if cmd == nil {
		t.Fatalf("expected first save command")
	}
	msg, ok := cmd().(appStateSavedMsg)
	if !ok {
		t.Fatalf("expected appStateSavedMsg, got %T", cmd())
	}
	if msg.requestSeq != 1 {
		t.Fatalf("expected request seq 1, got %d", msg.requestSeq)
	}

	m.appState.ActiveWorkspaceGroupIDs = []string{"group-2"}
	cmd = m.saveAppStateCmd()
	if cmd == nil {
		t.Fatalf("expected second save command")
	}
	msg, ok = cmd().(appStateSavedMsg)
	if !ok {
		t.Fatalf("expected appStateSavedMsg, got %T", cmd())
	}
	if msg.requestSeq != 2 {
		t.Fatalf("expected request seq 2, got %d", msg.requestSeq)
	}
}

func TestUpdateIgnoresStaleAppStateSavedMsg(t *testing.T) {
	m := NewModel(nil)
	m.appStateSaveSeq = 2
	m.appState.ActiveWorkspaceGroupIDs = []string{"group-new"}
	if m.menu == nil {
		t.Fatalf("expected menu controller")
	}
	m.menu.SetSelectedGroupIDs([]string{"group-new"})

	nextModel, cmd := m.Update(appStateSavedMsg{
		requestSeq: 1,
		state: &types.AppState{
			ActiveWorkspaceGroupIDs: []string{"group-old"},
		},
	})
	if cmd != nil {
		t.Fatalf("expected no command for stale save response")
	}
	next := asModel(t, nextModel)

	if !reflect.DeepEqual(next.appState.ActiveWorkspaceGroupIDs, []string{"group-new"}) {
		t.Fatalf("expected stale response to be ignored, got %#v", next.appState.ActiveWorkspaceGroupIDs)
	}
	if !reflect.DeepEqual(next.menu.SelectedGroupIDs(), []string{"group-new"}) {
		t.Fatalf("expected menu selection to stay on group-new, got %#v", next.menu.SelectedGroupIDs())
	}
}

func TestUpdateAppliesLatestAppStateSavedMsg(t *testing.T) {
	m := NewModel(nil)
	m.appStateSaveSeq = 2
	m.appState.ActiveWorkspaceGroupIDs = []string{"group-old"}
	if m.menu == nil {
		t.Fatalf("expected menu controller")
	}
	m.menu.SetSelectedGroupIDs([]string{"group-old"})

	nextModel, cmd := m.Update(appStateSavedMsg{
		requestSeq: 2,
		state: &types.AppState{
			ActiveWorkspaceGroupIDs: []string{"group-new"},
		},
	})
	if cmd != nil {
		t.Fatalf("expected no command for save response")
	}
	next := asModel(t, nextModel)

	if !reflect.DeepEqual(next.appState.ActiveWorkspaceGroupIDs, []string{"group-new"}) {
		t.Fatalf("expected latest save response to apply, got %#v", next.appState.ActiveWorkspaceGroupIDs)
	}
	if !reflect.DeepEqual(next.menu.SelectedGroupIDs(), []string{"group-new"}) {
		t.Fatalf("expected menu selection to update, got %#v", next.menu.SelectedGroupIDs())
	}
}

func TestUpdateIgnoresStaleAppStateSaveError(t *testing.T) {
	m := NewModel(nil)
	m.appStateSaveSeq = 3
	m.status = "steady"

	nextModel, cmd := m.Update(appStateSavedMsg{
		requestSeq: 2,
		err:        errors.New("stale write failed"),
	})
	if cmd != nil {
		t.Fatalf("expected no command for stale save error")
	}
	next := asModel(t, nextModel)

	if next.status != "steady" {
		t.Fatalf("expected stale save error to be ignored, got status %q", next.status)
	}
}
