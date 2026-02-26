package daemon

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type stubTurnArtifactRepository struct {
	readErr      error
	appendErr    error
	items        []map[string]any
	appendCalled int
}

func (s *stubTurnArtifactRepository) ReadItems(string, int) ([]map[string]any, error) {
	if s.readErr != nil {
		return nil, s.readErr
	}
	out := make([]map[string]any, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, cloneItemMap(item))
	}
	return out, nil
}

func (s *stubTurnArtifactRepository) AppendItems(_ string, items []map[string]any) error {
	s.appendCalled++
	if s.appendErr != nil {
		return s.appendErr
	}
	for _, item := range items {
		s.items = append(s.items, cloneItemMap(item))
	}
	return nil
}

type stubTurnArtifactRemote struct {
	errByDirectory map[string]error
	messages       []openCodeSessionMessage
	directories    []string
}

func (s *stubTurnArtifactRemote) ListSessionMessages(_ context.Context, _ string, directory string, _ int) ([]openCodeSessionMessage, error) {
	s.directories = append(s.directories, directory)
	if err := s.errByDirectory[directory]; err != nil {
		return nil, err
	}
	return s.messages, nil
}

func TestTurnArtifactSynchronizerBackfillsAndMarksReady(t *testing.T) {
	remote := &stubTurnArtifactRemote{
		messages: []openCodeSessionMessage{
			{
				Info: map[string]any{"role": "assistant"},
				Parts: []map[string]any{
					{"type": "text", "text": "final answer"},
				},
			},
		},
	}
	repo := &stubTurnArtifactRepository{
		items: []map[string]any{
			{
				"type": "userMessage",
				"content": []map[string]any{
					{"type": "text", "text": "hello"},
				},
			},
		},
	}
	syncer := newOpenCodeTurnArtifactSynchronizer("sess-1", "prov-1", "/tmp/repo", remote, repo)
	result := syncer.SyncTurnArtifacts(context.Background(), turnEventParams{})
	if !result.ArtifactsPersisted {
		t.Fatalf("expected artifacts persisted after backfill")
	}
	if result.AssistantArtifactCount != 1 {
		t.Fatalf("expected one assistant artifact, got %d", result.AssistantArtifactCount)
	}
	if result.Output != "final answer" {
		t.Fatalf("expected output from assistant artifact, got %q", result.Output)
	}
	if repo.appendCalled != 1 {
		t.Fatalf("expected one append call, got %d", repo.appendCalled)
	}
}

func TestTurnArtifactSynchronizerFallsBackToEmptyDirectory(t *testing.T) {
	remote := &stubTurnArtifactRemote{
		errByDirectory: map[string]error{
			"/tmp/repo": errors.New("directory scoped lookup failed"),
		},
		messages: []openCodeSessionMessage{
			{
				Info: map[string]any{"role": "assistant"},
				Parts: []map[string]any{
					{"type": "text", "text": "fallback answer"},
				},
			},
		},
	}
	repo := &stubTurnArtifactRepository{}
	syncer := newOpenCodeTurnArtifactSynchronizer("sess-1", "prov-1", "/tmp/repo", remote, repo)
	result := syncer.SyncTurnArtifacts(context.Background(), turnEventParams{})
	if strings.TrimSpace(result.Error) != "" {
		t.Fatalf("expected fallback to recover remote sync, got error %q", result.Error)
	}
	if len(remote.directories) < 2 || remote.directories[0] != "/tmp/repo" || remote.directories[1] != "" {
		t.Fatalf("expected fallback call sequence, got %#v", remote.directories)
	}
}

func TestTurnArtifactSynchronizerReturnsRemoteError(t *testing.T) {
	remote := &stubTurnArtifactRemote{
		errByDirectory: map[string]error{
			"": errors.New("remote unavailable"),
		},
	}
	repo := &stubTurnArtifactRepository{}
	syncer := newOpenCodeTurnArtifactSynchronizer("sess-1", "prov-1", "", remote, repo)
	result := syncer.SyncTurnArtifacts(context.Background(), turnEventParams{Output: "turn fallback"})
	if !strings.Contains(result.Error, "remote unavailable") {
		t.Fatalf("expected remote error in sync result, got %q", result.Error)
	}
	if result.Output != "turn fallback" {
		t.Fatalf("expected fallback output from turn params, got %q", result.Output)
	}
}

func TestTurnArtifactSynchronizerReturnsNopWhenMissingDependencies(t *testing.T) {
	syncer := newOpenCodeTurnArtifactSynchronizer(" ", "prov-1", "", &stubTurnArtifactRemote{}, &stubTurnArtifactRepository{})
	result := syncer.SyncTurnArtifacts(context.Background(), turnEventParams{Output: "hello"})
	if result.Source != "noop" {
		t.Fatalf("expected noop synchronizer, got source %q", result.Source)
	}
	if result.Output != "hello" {
		t.Fatalf("expected turn output passthrough, got %q", result.Output)
	}
}

func TestDefaultTurnCompletionPayloadBuilderUsesTurnOutputFallback(t *testing.T) {
	builder := defaultTurnCompletionPayloadBuilder{}
	output, payload := builder.Build(turnEventParams{Output: "turn output"}, TurnArtifactSyncResult{
		Source: "sync",
	})
	if output != "turn output" {
		t.Fatalf("expected turn output fallback, got %q", output)
	}
	if strings.TrimSpace(asString(payload["turn_output"])) != "turn output" {
		t.Fatalf("expected turn_output payload field, got %#v", payload)
	}
	if strings.TrimSpace(asString(payload["artifact_sync_source"])) != "sync" {
		t.Fatalf("expected artifact_sync_source field, got %#v", payload)
	}
}

func TestOpenCodeTurnArtifactRemoteSourceNilClient(t *testing.T) {
	source := openCodeTurnArtifactRemoteSource{client: nil}
	messages, err := source.ListSessionMessages(context.Background(), "sess-1", "", 10)
	if err != nil {
		t.Fatalf("expected nil error for nil client, got %v", err)
	}
	if len(messages) != 0 {
		t.Fatalf("expected no messages for nil client, got %#v", messages)
	}
}

func TestOpenCodeTurnArtifactRemoteSourceDelegatesToClient(t *testing.T) {
	source := openCodeTurnArtifactRemoteSource{client: &openCodeClient{}}
	_, err := source.ListSessionMessages(context.Background(), "sess-1", "", 10)
	if err == nil {
		t.Fatalf("expected client delegation error when client session service is unavailable")
	}
}
