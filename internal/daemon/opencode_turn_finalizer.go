package daemon

import (
	"context"
	"strings"
	"sync"
)

type openCodeTurnFinalizer interface {
	FinalizeTurn(turn turnEventParams, additionalPayload map[string]any)
}

type defaultOpenCodeTurnFinalizer struct {
	mu           sync.Mutex
	sessionID    string
	providerName string
	notifier     TurnCompletionNotifier
	artifactSync TurnArtifactSynchronizer
	payloads     TurnCompletionPayloadBuilder
	freshness    TurnEvidenceFreshnessTracker
}

func (f *defaultOpenCodeTurnFinalizer) FinalizeTurn(turn turnEventParams, additionalPayload map[string]any) {
	if f == nil || f.notifier == nil {
		return
	}
	syncResult := TurnArtifactSyncResult{}
	if f.artifactSync != nil {
		syncResult = f.artifactSync.SyncTurnArtifacts(context.Background(), turn)
	}
	output := strings.TrimSpace(syncResult.Output)
	payload := map[string]any{}
	if f.payloads != nil {
		output, payload = f.payloads.Build(turn, syncResult)
	} else {
		output, payload = defaultTurnCompletionPayloadBuilder{}.Build(turn, syncResult)
	}
	freshness := f.freshnessOrDefault()
	fresh := freshness.MarkFresh(f.sessionID, syncResult.AssistantEvidenceKey, output)
	if payload == nil {
		payload = map[string]any{}
	}
	for key, value := range additionalPayload {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			continue
		}
		payload[trimmed] = value
	}
	payload["turn_output_fresh"] = fresh
	if !fresh {
		output = ""
		delete(payload, "turn_output")
		payload["stale_turn_output_dropped"] = true
	}
	f.notifier.NotifyTurnCompletedEvent(context.Background(), TurnCompletionEvent{
		SessionID: strings.TrimSpace(f.sessionID),
		TurnID:    strings.TrimSpace(turn.TurnID),
		Provider:  strings.TrimSpace(f.providerName),
		Source:    "live_session_event",
		Status:    strings.TrimSpace(turn.Status),
		Error:     strings.TrimSpace(turn.Error),
		Output:    strings.TrimSpace(output),
		Payload:   payload,
	})
}

func (f *defaultOpenCodeTurnFinalizer) freshnessOrDefault() TurnEvidenceFreshnessTracker {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.freshness == nil {
		f.freshness = NewTurnEvidenceFreshnessTracker()
	}
	return f.freshness
}
