package app

import (
	"testing"
	"time"
)

func TestDefaultProviderMessageIdentityResolverPrefersExplicitProviderIDs(t *testing.T) {
	resolver := defaultProviderMessageIdentityResolver{}
	item := map[string]any{
		"type":                "assistant",
		"provider_message_id": "msg-1",
		"id":                  "generic-id",
	}
	if got := resolver.Resolve(item, "assistant"); got != "msg-1" {
		t.Fatalf("expected explicit provider id, got %q", got)
	}
}

func TestDefaultProviderMessageIdentityResolverSkipsGenericIDForNonAssistantItem(t *testing.T) {
	resolver := defaultProviderMessageIdentityResolver{}
	item := map[string]any{
		"type": "userMessage",
		"id":   "user-id-1",
	}
	if got := resolver.Resolve(item, "userMessage"); got != "" {
		t.Fatalf("expected empty id for non-assistant generic id, got %q", got)
	}
}

func TestDefaultProviderMessageIdentityResolverReadsNestedExplicitIDs(t *testing.T) {
	resolver := defaultProviderMessageIdentityResolver{}
	fromMessage := map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"providerMessageID": "msg-nested",
		},
	}
	if got := resolver.Resolve(fromMessage, "assistant"); got != "msg-nested" {
		t.Fatalf("expected nested message provider id, got %q", got)
	}

	fromInfo := map[string]any{
		"type": "assistant",
		"info": map[string]any{
			"messageID": "msg-info",
		},
	}
	if got := resolver.Resolve(fromInfo, "assistant"); got != "msg-info" {
		t.Fatalf("expected nested info provider id, got %q", got)
	}
}

func TestDefaultProviderMessageIdentityResolverAssistantFallbackGenericIDs(t *testing.T) {
	resolver := defaultProviderMessageIdentityResolver{}

	if got := resolver.Resolve(map[string]any{"type": "assistant", "id": "assistant-id"}, "assistant"); got != "assistant-id" {
		t.Fatalf("expected assistant generic id fallback, got %q", got)
	}
	if got := resolver.Resolve(map[string]any{"type": "result", "item_id": "result-item"}, "result"); got != "result-item" {
		t.Fatalf("expected result item_id fallback, got %q", got)
	}
	if got := resolver.Resolve(map[string]any{"type": "assistant", "message": map[string]any{"id": "msg-inner"}}, "assistant"); got != "msg-inner" {
		t.Fatalf("expected nested message id fallback, got %q", got)
	}
	if got := resolver.Resolve(map[string]any{"type": "assistant", "info": map[string]any{"id": "info-inner"}}, "assistant"); got != "info-inner" {
		t.Fatalf("expected nested info id fallback, got %q", got)
	}
}

func TestDefaultAssistantItemMetadataExtractorExtractsTurnAndProviderID(t *testing.T) {
	extractor := defaultAssistantItemMetadataExtractor{
		identityResolver: defaultProviderMessageIdentityResolver{},
	}
	meta := extractor.Extract(map[string]any{
		"type":                "assistant",
		"turn_id":             "turn-1",
		"provider_message_id": "msg-1",
	}, time.Now().UTC(), "assistant")
	if meta.turnID != "turn-1" {
		t.Fatalf("expected turn-1, got %q", meta.turnID)
	}
	if meta.providerMessageID != "msg-1" {
		t.Fatalf("expected msg-1, got %q", meta.providerMessageID)
	}
}

func TestDefaultAssistantItemMetadataExtractorNormalizesEmptyItemTypeFromPayload(t *testing.T) {
	extractor := defaultAssistantItemMetadataExtractor{
		identityResolver: defaultProviderMessageIdentityResolver{},
	}
	meta := extractor.Extract(map[string]any{
		"type": "assistant",
		"id":   "fallback-id",
	}, time.Now().UTC(), "")
	if meta.providerMessageID != "fallback-id" {
		t.Fatalf("expected fallback id from item type resolution, got %q", meta.providerMessageID)
	}
}

func TestDefaultAssistantItemMetadataExtractorUsesDefaultResolverWhenNil(t *testing.T) {
	extractor := defaultAssistantItemMetadataExtractor{}
	meta := extractor.Extract(map[string]any{
		"type": "assistant",
		"id":   "assistant-id",
	}, time.Now().UTC(), "assistant")
	if meta.providerMessageID != "assistant-id" {
		t.Fatalf("expected default resolver fallback id, got %q", meta.providerMessageID)
	}
}
