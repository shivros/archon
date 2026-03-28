package app

import (
	"context"
	"errors"
	"testing"

	"control/internal/types"
)

func TestDefaultComposeFileSearchCoordinatorSyncInputLifecycle(t *testing.T) {
	coordinator, ok := NewDefaultComposeFileSearchCoordinator().(*defaultComposeFileSearchCoordinator)
	if !ok {
		t.Fatalf("expected default compose file search coordinator")
	}

	fragment := composeFileSearchFragment{Start: 0, End: 3, Query: "ma"}
	transition := coordinator.SyncInput("scope-a", fragment, true)
	if !transition.ReplaceLifecycleScope || !transition.ScheduleDebounce || transition.Seq != 1 {
		t.Fatalf("expected initial lifecycle transition to replace scope and schedule debounce, got %#v", transition)
	}
	if !transition.LoadingChanged || !transition.Loading {
		t.Fatalf("expected initial lifecycle transition to enable loading, got %#v", transition)
	}

	transition = coordinator.SyncInput("scope-a", fragment, true)
	if transition.ScheduleDebounce {
		t.Fatalf("did not expect unchanged fragment to reschedule debounce: %#v", transition)
	}

	coordinator.RememberSearchID("fs-1")
	nextFragment := composeFileSearchFragment{Start: 0, End: 4, Query: "main"}
	transition = coordinator.SyncInput("scope-a", nextFragment, true)
	if !transition.EnsureLifecycleScope || !transition.ScheduleDebounce || transition.Seq != 2 {
		t.Fatalf("expected same lifecycle query update to schedule debounce, got %#v", transition)
	}

	transition = coordinator.SyncInput("scope-b", nextFragment, true)
	if !transition.ReplaceLifecycleScope || transition.SearchIDToClose != "fs-1" {
		t.Fatalf("expected scope change to replace lifecycle and close prior search, got %#v", transition)
	}
	if transition.Seq != 1 {
		t.Fatalf("expected scope change to restart request sequencing after reset, got %d", transition.Seq)
	}
}

func TestDefaultComposeFileSearchCoordinatorStaleSuppressionAndEvents(t *testing.T) {
	coordinator, ok := NewDefaultComposeFileSearchCoordinator().(*defaultComposeFileSearchCoordinator)
	if !ok {
		t.Fatalf("expected default compose file search coordinator")
	}

	fragment := composeFileSearchFragment{Start: 0, End: 3, Query: "ma"}
	syncTransition := coordinator.SyncInput("scope-a", fragment, true)
	if syncTransition.Seq != 1 {
		t.Fatalf("expected first request seq to be 1, got %d", syncTransition.Seq)
	}

	staleStart := coordinator.ApplyStarted(syncTransition.Seq+1, "ma", composeFileSearchStartResult{
		Session: &types.FileSearchSession{ID: "stale-search"},
	})
	if staleStart.SearchIDToClose != "stale-search" || staleStart.OpenStream {
		t.Fatalf("expected stale start to be rejected and closed, got %#v", staleStart)
	}

	started := coordinator.ApplyStarted(syncTransition.Seq, "ma", composeFileSearchStartResult{
		Session: &types.FileSearchSession{ID: "fs-2"},
	})
	if !started.OpenStream || started.SearchID != "fs-2" {
		t.Fatalf("expected matching start to open stream, got %#v", started)
	}

	eventTransition := coordinator.ApplyEvent(types.FileSearchEvent{
		Kind:       types.FileSearchEventResults,
		SearchID:   "fs-2",
		Query:      "other",
		Candidates: []types.FileSearchCandidate{{Path: "/repo/ignored.go", DisplayPath: "ignored.go"}},
	})
	if eventTransition.ApplyCandidates {
		t.Fatalf("did not expect stale query event to apply candidates: %#v", eventTransition)
	}

	eventTransition = coordinator.ApplyEvent(types.FileSearchEvent{
		Kind:       types.FileSearchEventResults,
		SearchID:   "fs-2",
		Query:      "ma",
		Candidates: []types.FileSearchCandidate{{Path: "/repo/main.go", DisplayPath: "main.go"}},
	})
	if !eventTransition.ApplyCandidates || len(eventTransition.Candidates) != 1 {
		t.Fatalf("expected matching event to apply candidates, got %#v", eventTransition)
	}
	if !eventTransition.LoadingChanged || eventTransition.Loading {
		t.Fatalf("expected matching event to clear loading, got %#v", eventTransition)
	}
}

func TestDefaultComposeFileSearchCoordinatorAsyncBranches(t *testing.T) {
	t.Run("prepare debounce rejects stale and empty", func(t *testing.T) {
		coordinator := &defaultComposeFileSearchCoordinator{}
		if _, ok := coordinator.PrepareDebounce(1); ok {
			t.Fatalf("expected prepare debounce to reject stale seq with no active fragment")
		}

		coordinator.SyncInput("scope-a", composeFileSearchFragment{Start: 0, End: 1, Query: ""}, true)
		transition, ok := coordinator.PrepareDebounce(1)
		if ok {
			t.Fatalf("expected stale seq to be rejected, got %#v", transition)
		}

		coordinator.requestSeq = 2
		coordinator.fragmentOpen = true
		coordinator.fragment = composeFileSearchFragment{Start: 0, End: 1, Query: "   "}
		transition, ok = coordinator.PrepareDebounce(2)
		if !ok || !transition.LoadingChanged || transition.Loading || transition.Query != "" {
			t.Fatalf("expected empty query debounce transition, got %#v %v", transition, ok)
		}
	})

	t.Run("apply started handles canceled and unsupported", func(t *testing.T) {
		coordinator := &defaultComposeFileSearchCoordinator{
			searchID:     "fs-active",
			fragment:     composeFileSearchFragment{Start: 0, End: 3, Query: "ma"},
			fragmentOpen: true,
			requestSeq:   1,
		}
		canceled := coordinator.ApplyStarted(1, "ma", composeFileSearchStartResult{
			Session: &types.FileSearchSession{ID: "fs-canceled"},
			Err:     context.Canceled,
		})
		if canceled.SearchIDToClose != "fs-canceled" || canceled.Unsupported {
			t.Fatalf("expected canceled start to close stale session only, got %#v", canceled)
		}

		unsupported := coordinator.ApplyStarted(1, "ma", composeFileSearchStartResult{
			Session:     &types.FileSearchSession{ID: "fs-unsupported"},
			Unsupported: true,
			Err:         errors.New("unsupported"),
		})
		if !unsupported.Unsupported || !unsupported.Reset || unsupported.SearchIDToClose != "fs-active" {
			t.Fatalf("expected unsupported start to reset active lifecycle, got %#v", unsupported)
		}
		if coordinator.CurrentSearchID() != "" || coordinator.CurrentRequestSeq() != 0 {
			t.Fatalf("expected coordinator reset after unsupported start, got search=%q seq=%d", coordinator.CurrentSearchID(), coordinator.CurrentRequestSeq())
		}
	})

	t.Run("apply updated handles stale canceled and unsupported", func(t *testing.T) {
		coordinator := &defaultComposeFileSearchCoordinator{
			searchID:     "fs-1",
			fragment:     composeFileSearchFragment{Start: 0, End: 3, Query: "ma"},
			fragmentOpen: true,
			requestSeq:   2,
		}
		stale := coordinator.ApplyUpdated(1, "ma", composeFileSearchUpdateResult{
			SearchID: "fs-stale",
		})
		if stale != (composeFileSearchAsyncTransition{}) {
			t.Fatalf("expected stale update to be ignored, got %#v", stale)
		}

		canceled := coordinator.ApplyUpdated(2, "ma", composeFileSearchUpdateResult{
			SearchID: "fs-1",
			Err:      context.Canceled,
		})
		if canceled != (composeFileSearchAsyncTransition{}) {
			t.Fatalf("expected canceled update to be ignored, got %#v", canceled)
		}

		unsupported := coordinator.ApplyUpdated(2, "ma", composeFileSearchUpdateResult{
			SearchID:    "fs-1",
			Unsupported: true,
			Err:         errors.New("unsupported"),
		})
		if !unsupported.Unsupported || !unsupported.Reset || unsupported.SearchIDToClose != "fs-1" {
			t.Fatalf("expected unsupported update to reset current lifecycle, got %#v", unsupported)
		}
	})

	t.Run("apply stream open handles accept mismatch and error", func(t *testing.T) {
		coordinator := &defaultComposeFileSearchCoordinator{
			searchID:     "fs-1",
			fragment:     composeFileSearchFragment{Start: 0, End: 3, Query: "ma"},
			fragmentOpen: true,
			requestSeq:   1,
		}
		accepted := coordinator.ApplyStreamOpen("fs-1", nil)
		if !accepted.Accept || accepted.SearchIDToClose != "" {
			t.Fatalf("expected matching stream open to be accepted, got %#v", accepted)
		}

		mismatch := coordinator.ApplyStreamOpen("fs-other", nil)
		if mismatch.Accept || mismatch.SearchIDToClose != "fs-other" {
			t.Fatalf("expected mismatched stream open to be closed, got %#v", mismatch)
		}

		canceled := coordinator.ApplyStreamOpen("fs-1", context.Canceled)
		if canceled != (composeFileSearchStreamTransition{}) {
			t.Fatalf("expected canceled stream open to be ignored, got %#v", canceled)
		}

		errTransition := coordinator.ApplyStreamOpen("fs-1", errors.New("boom"))
		if !errTransition.Reset || errTransition.SearchIDToClose != "fs-1" || !errTransition.ClosePopup {
			t.Fatalf("expected errored stream open to reset lifecycle, got %#v", errTransition)
		}
	})

	t.Run("apply event handles fail close and missing fragment", func(t *testing.T) {
		coordinator := &defaultComposeFileSearchCoordinator{
			searchID:     "fs-1",
			fragment:     composeFileSearchFragment{Start: 0, End: 3, Query: "ma"},
			fragmentOpen: true,
			requestSeq:   1,
		}
		failed := coordinator.ApplyEvent(types.FileSearchEvent{
			Kind:     types.FileSearchEventFailed,
			SearchID: "fs-1",
			Query:    "ma",
			Error:    "boom",
		})
		if !failed.ClosePopup || !failed.LoadingChanged || failed.Loading {
			t.Fatalf("expected failed event to close popup and clear loading, got %#v", failed)
		}

		closed := coordinator.ApplyEvent(types.FileSearchEvent{
			Kind:     types.FileSearchEventClosed,
			SearchID: "fs-1",
		})
		if !closed.LoadingChanged || closed.Loading {
			t.Fatalf("expected closed event to clear loading, got %#v", closed)
		}

		coordinator.fragmentOpen = false
		missing := coordinator.ApplyEvent(types.FileSearchEvent{
			Kind:     types.FileSearchEventResults,
			SearchID: "fs-1",
			Query:    "ma",
		})
		if !missing.Reset || missing.SearchIDToClose != "fs-1" {
			t.Fatalf("expected missing fragment to reset and close active search, got %#v", missing)
		}
	})
}

type stubComposeFileSearchCloser struct {
	searchIDs []string
}

func (s *stubComposeFileSearchCloser) CloseAsync(_ composeFileSearchService, searchID string) {
	s.searchIDs = append(s.searchIDs, searchID)
}

func TestComposeFileSearchCoordinatorOptionsAndFallbacks(t *testing.T) {
	t.Run("coordinator option installs and nil falls back", func(t *testing.T) {
		custom := &defaultComposeFileSearchCoordinator{searchID: "custom"}
		m := NewModel(nil, WithComposeFileSearchCoordinator(custom))
		if got, ok := m.composeFileSearchCoordinatorOrDefault().(*defaultComposeFileSearchCoordinator); !ok || got != custom {
			t.Fatalf("expected custom coordinator to be preserved, got %T %#v", m.composeFileSearchCoordinatorOrDefault(), got)
		}

		mFallback := NewModel(nil, WithComposeFileSearchCoordinator(nil))
		if mFallback.composeFileSearchCoordinatorOrDefault() == nil {
			t.Fatalf("expected nil coordinator option to fall back to default")
		}
	})

	t.Run("closer option installs and nil falls back", func(t *testing.T) {
		closer := &stubComposeFileSearchCloser{}
		m := NewModel(nil, WithComposeFileSearchCloser(closer))
		if got, ok := m.composeFileSearchCloserOrDefault().(*stubComposeFileSearchCloser); !ok || got != closer {
			t.Fatalf("expected custom closer to be preserved, got %T %#v", m.composeFileSearchCloserOrDefault(), got)
		}

		mFallback := NewModel(nil, WithComposeFileSearchCloser(nil))
		if mFallback.composeFileSearchCloserOrDefault() == nil {
			t.Fatalf("expected nil closer option to fall back to default")
		}
	})
}

func TestComposeFileSearchPresenterHelpers(t *testing.T) {
	options := composeFileSearchPickerOptions(nil, map[string]types.FileSearchCandidate{}, true)
	if len(options) != 1 || options[0].label != " (loading files...)" {
		t.Fatalf("expected loading placeholder option, got %#v", options)
	}

	options = composeFileSearchPickerOptions(
		[]string{"/repo/main.go"},
		map[string]types.FileSearchCandidate{
			"/repo/main.go": {Path: "/repo/main.go", DisplayPath: "main.go"},
		},
		false,
	)
	if len(options) != 1 || options[0].id != "/repo/main.go" || options[0].label != "main.go" {
		t.Fatalf("expected presenter to preserve candidate option identity, got %#v", options)
	}

	if got := composeFileSearchSupplementalView(true, 0); got != "" {
		t.Fatalf("did not expect supplemental loading text without visible results, got %q", got)
	}
	if got := composeFileSearchSupplementalView(true, 1); got != " loading files..." {
		t.Fatalf("expected supplemental loading text with visible results, got %q", got)
	}
}
