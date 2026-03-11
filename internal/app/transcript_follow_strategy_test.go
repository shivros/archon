package app

import "testing"

type followStrategyRegistryStub struct{}

func (followStrategyRegistryStub) StrategyFor(TranscriptAttachmentSource) TranscriptFollowStrategy {
	return staticTranscriptFollowStrategy{open: true, reconnectSource: transcriptSourceSessionBlocksProject}
}

func TestDefaultTranscriptFollowStrategyRegistryDecisions(t *testing.T) {
	registry := NewDefaultTranscriptFollowStrategyRegistry()

	selection := registry.StrategyFor(transcriptAttachmentSourceSelectionLoad).Decide(TranscriptFollowOpenRequest{
		SessionID:     "s1",
		Source:        transcriptAttachmentSourceSelectionLoad,
		AfterRevision: "12",
	})
	if !selection.Open || selection.ReconnectSource != transcriptSourceSessionBlocksProject {
		t.Fatalf("unexpected selection follow decision: %#v", selection)
	}

	recovery := registry.StrategyFor(transcriptAttachmentSourceRecovery).Decide(TranscriptFollowOpenRequest{
		SessionID:     "s1",
		Source:        transcriptAttachmentSourceRecovery,
		AfterRevision: "12",
	})
	if !recovery.Open || recovery.ReconnectSource != transcriptSourceAutoRefreshHistory {
		t.Fatalf("unexpected recovery follow decision: %#v", recovery)
	}

	unknown := registry.StrategyFor(transcriptAttachmentSourceManualRefresh).Decide(TranscriptFollowOpenRequest{
		SessionID:     "s1",
		Source:        transcriptAttachmentSourceManualRefresh,
		AfterRevision: "12",
	})
	if unknown.Open {
		t.Fatalf("expected manual-refresh follow to be disabled by default, got %#v", unknown)
	}
}

func TestWithTranscriptFollowStrategyRegistryOption(t *testing.T) {
	custom := &followStrategyRegistryStub{}
	model := NewModel(nil, WithTranscriptFollowStrategyRegistry(custom))
	if model.transcriptFollowStrategyRegistry != custom {
		t.Fatalf("expected custom follow strategy registry to be installed")
	}

	model = NewModel(nil, WithTranscriptFollowStrategyRegistry(nil))
	if model.transcriptFollowStrategyRegistry == nil {
		t.Fatalf("expected nil option to install default follow strategy registry")
	}
	if model.transcriptFollowStrategyRegistryOrDefault() == nil {
		t.Fatalf("expected follow strategy registry fallback")
	}
}
