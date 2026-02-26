package daemon

import (
	"context"
	"strings"
)

type TurnArtifactSyncResult struct {
	Output                 string
	ArtifactsPersisted     bool
	AssistantArtifactCount int
	Source                 string
	Error                  string
}

type TurnArtifactSynchronizer interface {
	SyncTurnArtifacts(ctx context.Context, turn turnEventParams) TurnArtifactSyncResult
}

type TurnArtifactRepository interface {
	ReadItems(sessionID string, lines int) ([]map[string]any, error)
	AppendItems(sessionID string, items []map[string]any) error
}

type TurnArtifactRemoteSource interface {
	ListSessionMessages(ctx context.Context, sessionID, directory string, limit int) ([]openCodeSessionMessage, error)
}

type TurnCompletionPayloadBuilder interface {
	Build(turn turnEventParams, syncResult TurnArtifactSyncResult) (string, map[string]any)
}

type nopTurnArtifactSynchronizer struct{}

func (nopTurnArtifactSynchronizer) SyncTurnArtifacts(_ context.Context, turn turnEventParams) TurnArtifactSyncResult {
	return TurnArtifactSyncResult{
		Output: strings.TrimSpace(turn.Output),
		Source: "noop",
	}
}

type openCodeTurnArtifactSynchronizer struct {
	sessionID  string
	providerID string
	directory  string
	remote     TurnArtifactRemoteSource
	repository TurnArtifactRepository
}

func newOpenCodeTurnArtifactSynchronizer(
	sessionID string,
	providerID string,
	directory string,
	remote TurnArtifactRemoteSource,
	repository TurnArtifactRepository,
) TurnArtifactSynchronizer {
	if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(providerID) == "" || remote == nil || repository == nil {
		return nopTurnArtifactSynchronizer{}
	}
	return &openCodeTurnArtifactSynchronizer{
		sessionID:  strings.TrimSpace(sessionID),
		providerID: strings.TrimSpace(providerID),
		directory:  strings.TrimSpace(directory),
		remote:     remote,
		repository: repository,
	}
}

func (s *openCodeTurnArtifactSynchronizer) SyncTurnArtifacts(ctx context.Context, turn turnEventParams) TurnArtifactSyncResult {
	if s == nil || s.remote == nil || s.repository == nil || strings.TrimSpace(s.providerID) == "" || strings.TrimSpace(s.sessionID) == "" {
		return TurnArtifactSyncResult{
			Output: strings.TrimSpace(turn.Output),
			Source: "unavailable",
		}
	}
	if ctx == nil {
		ctx = context.Background()
	}

	result := TurnArtifactSyncResult{
		Output: strings.TrimSpace(turn.Output),
		Source: "opencode_history_reconcile",
	}

	limit := 200
	messages, err := s.listSessionMessages(ctx, limit)
	if err != nil {
		result.Error = strings.TrimSpace(err.Error())
		return result
	}

	remoteItems := trimItemsToLimit(openCodeSessionMessagesToItems(messages), limit)
	localItems, readErr := s.repository.ReadItems(s.sessionID, limit)
	if readErr != nil {
		result.Error = strings.TrimSpace(readErr.Error())
	}

	missing := openCodeMissingHistoryItems(localItems, remoteItems)
	if len(missing) > 0 {
		if appendErr := s.repository.AppendItems(s.sessionID, missing); appendErr != nil {
			result.Error = strings.TrimSpace(appendErr.Error())
		}
	}

	if persistedItems, persistedErr := s.repository.ReadItems(s.sessionID, limit); persistedErr == nil {
		localItems = persistedItems
	}

	output, assistantCount := latestAssistantArtifact(remoteItems)
	if output == "" {
		output, assistantCount = latestAssistantArtifact(localItems)
	}
	if output != "" {
		result.Output = output
	}
	result.AssistantArtifactCount = assistantCount
	result.ArtifactsPersisted = hasAssistantArtifact(localItems)
	return result
}

func (s *openCodeTurnArtifactSynchronizer) listSessionMessages(ctx context.Context, limit int) ([]openCodeSessionMessage, error) {
	if s == nil || s.remote == nil {
		return nil, nil
	}
	messages, err := s.remote.ListSessionMessages(ctx, s.providerID, s.directory, limit)
	if err == nil {
		return messages, nil
	}
	if strings.TrimSpace(s.directory) == "" {
		return nil, err
	}
	return s.remote.ListSessionMessages(ctx, s.providerID, "", limit)
}

func latestAssistantArtifact(items []map[string]any) (string, int) {
	assistantCount := 0
	latest := ""
	for _, item := range items {
		if strings.ToLower(strings.TrimSpace(asString(item["type"]))) != "assistant" {
			continue
		}
		assistantCount++
		if text := strings.TrimSpace(openCodeHistoryItemText(item)); text != "" {
			latest = text
		}
	}
	return latest, assistantCount
}

func hasAssistantArtifact(items []map[string]any) bool {
	for _, item := range items {
		if strings.ToLower(strings.TrimSpace(asString(item["type"]))) != "assistant" {
			continue
		}
		if strings.TrimSpace(openCodeHistoryItemText(item)) != "" {
			return true
		}
	}
	return false
}

type defaultTurnCompletionPayloadBuilder struct{}

func (defaultTurnCompletionPayloadBuilder) Build(turn turnEventParams, syncResult TurnArtifactSyncResult) (string, map[string]any) {
	output := strings.TrimSpace(syncResult.Output)
	if output == "" {
		output = strings.TrimSpace(turn.Output)
	}
	payload := map[string]any{
		"artifacts_persisted":      syncResult.ArtifactsPersisted,
		"assistant_artifact_count": syncResult.AssistantArtifactCount,
		"artifact_sync_source":     strings.TrimSpace(syncResult.Source),
	}
	if errMsg := strings.TrimSpace(syncResult.Error); errMsg != "" {
		payload["artifact_sync_error"] = errMsg
	}
	if output != "" {
		payload["turn_output"] = output
	}
	return output, payload
}

type openCodeTurnArtifactRemoteSource struct {
	client *openCodeClient
}

func (s openCodeTurnArtifactRemoteSource) ListSessionMessages(
	ctx context.Context,
	sessionID string,
	directory string,
	limit int,
) ([]openCodeSessionMessage, error) {
	if s.client == nil {
		return nil, nil
	}
	return s.client.ListSessionMessages(ctx, sessionID, directory, limit)
}
