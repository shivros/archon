package app

import (
	"strings"
	"time"
)

type assistantItemMetadata struct {
	turnID            string
	providerMessageID string
}

type assistantItemMetadataExtractor interface {
	Extract(item map[string]any, createdAt time.Time, itemType string) assistantItemMetadata
}

type providerMessageIdentityResolver interface {
	Resolve(item map[string]any, itemType string) string
}

type defaultAssistantItemMetadataExtractor struct {
	identityResolver providerMessageIdentityResolver
}

func (e defaultAssistantItemMetadataExtractor) Extract(item map[string]any, _ time.Time, itemType string) assistantItemMetadata {
	if item == nil {
		return assistantItemMetadata{}
	}
	if strings.TrimSpace(itemType) == "" {
		itemType = strings.TrimSpace(asString(item["type"]))
	}
	return assistantItemMetadata{
		turnID:            itemTurnID(item),
		providerMessageID: strings.TrimSpace(e.identityResolverOrDefault().Resolve(item, itemType)),
	}
}

func (e defaultAssistantItemMetadataExtractor) identityResolverOrDefault() providerMessageIdentityResolver {
	if e.identityResolver == nil {
		return defaultProviderMessageIdentityResolver{}
	}
	return e.identityResolver
}

type defaultProviderMessageIdentityResolver struct{}

func (defaultProviderMessageIdentityResolver) Resolve(item map[string]any, itemType string) string {
	if item == nil {
		return ""
	}
	// Prefer explicit provider message identity keys.
	for _, key := range []string{"provider_message_id", "providerMessageID", "message_id", "messageID"} {
		if id := strings.TrimSpace(asString(item[key])); id != "" {
			return id
		}
	}
	if msg, ok := item["message"].(map[string]any); ok && msg != nil {
		for _, key := range []string{"provider_message_id", "providerMessageID", "message_id", "messageID"} {
			if id := strings.TrimSpace(asString(msg[key])); id != "" {
				return id
			}
		}
	}
	if info, ok := item["info"].(map[string]any); ok && info != nil {
		for _, key := range []string{"provider_message_id", "providerMessageID", "message_id", "messageID"} {
			if id := strings.TrimSpace(asString(info[key])); id != "" {
				return id
			}
		}
	}

	// Fallback to generic IDs only for assistant-like item families.
	switch strings.ToLower(strings.TrimSpace(itemType)) {
	case "assistant", "agentmessage", "result":
		for _, key := range []string{"id", "item_id"} {
			if id := strings.TrimSpace(asString(item[key])); id != "" {
				return id
			}
		}
		if msg, ok := item["message"].(map[string]any); ok && msg != nil {
			if id := strings.TrimSpace(asString(msg["id"])); id != "" {
				return id
			}
		}
		if info, ok := item["info"].(map[string]any); ok && info != nil {
			if id := strings.TrimSpace(asString(info["id"])); id != "" {
				return id
			}
		}
	}
	return ""
}
