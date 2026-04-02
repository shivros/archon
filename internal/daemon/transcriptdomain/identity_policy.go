package transcriptdomain

import (
	"encoding/json"
	"strings"
	"time"
)

type MessageIdentityScope string

const (
	MessageIdentityScopeNone            MessageIdentityScope = ""
	MessageIdentityScopeProviderMessage MessageIdentityScope = "provider_message_id"
	MessageIdentityScopeProviderItem    MessageIdentityScope = "provider_item_id"
	MessageIdentityScopeTurnScoped      MessageIdentityScope = "turn_scoped_id"
)

type MessageIdentity struct {
	Role              string
	ProviderMessageID string
	ProviderItemID    string
	TurnID            string
	TurnScopedID      string
	Scope             MessageIdentityScope
	Value             string
}

func (m MessageIdentity) HasStableIdentity() bool {
	return strings.TrimSpace(m.Value) != "" && m.Scope != MessageIdentityScopeNone
}

type TranscriptIdentityBlock struct {
	ID                string
	Kind              string
	Role              string
	Text              string
	TurnID            string
	ProviderMessageID string
	CreatedAt         time.Time
	Meta              map[string]any
}

type TranscriptIdentityPolicy interface {
	Identity(block TranscriptIdentityBlock) MessageIdentity
	Equivalent(a, b TranscriptIdentityBlock) bool
	CanFinalizeReplace(current, candidate TranscriptIdentityBlock) bool
}

type defaultTranscriptIdentityPolicy struct{}

func NewDefaultTranscriptIdentityPolicy() TranscriptIdentityPolicy {
	return defaultTranscriptIdentityPolicy{}
}

func (defaultTranscriptIdentityPolicy) Identity(block TranscriptIdentityBlock) MessageIdentity {
	meta := block.Meta
	role := strings.ToLower(strings.TrimSpace(block.Role))
	providerMessageID := firstIdentityValue(
		block.ProviderMessageID,
		identityMetaString(meta, "provider_message_id", "providerMessageID", "message_id", "messageId"),
	)
	providerItemID := firstIdentityValue(
		block.ID,
		identityMetaString(meta, "item_id", "itemId", "itemid", "id"),
	)
	turnID := firstIdentityValue(
		block.TurnID,
		identityMetaString(meta, "turn_id", "turnId", "turnID"),
	)
	turnScopedID := identityMetaString(meta,
		"turn_scoped_id",
		"turnScopedID",
		"turn_item_id",
		"turnItemID",
		"turn_item_index",
		"turnItemIndex",
		"turn_index",
		"turnIndex",
		"index",
	)

	identity := MessageIdentity{
		Role:              role,
		ProviderMessageID: providerMessageID,
		ProviderItemID:    providerItemID,
		TurnID:            turnID,
		TurnScopedID:      turnScopedID,
	}
	switch {
	case providerMessageID != "":
		identity.Scope = MessageIdentityScopeProviderMessage
		identity.Value = providerMessageID
	case providerItemID != "":
		identity.Scope = MessageIdentityScopeProviderItem
		identity.Value = providerItemID
	case turnID != "" && turnScopedID != "":
		identity.Scope = MessageIdentityScopeTurnScoped
		identity.Value = turnID + "::" + turnScopedID
	}
	return identity
}

func (p defaultTranscriptIdentityPolicy) Equivalent(a, b TranscriptIdentityBlock) bool {
	identityA := p.Identity(a)
	identityB := p.Identity(b)
	if !identityRolesCompatible(identityA.Role, identityB.Role) {
		return false
	}
	if identityA.ProviderMessageID != "" && identityB.ProviderMessageID != "" {
		return identityA.ProviderMessageID == identityB.ProviderMessageID
	}
	if identityA.ProviderItemID != "" && identityB.ProviderItemID != "" {
		return identityA.ProviderItemID == identityB.ProviderItemID
	}
	if identityA.TurnID != "" && identityB.TurnID != "" &&
		identityA.TurnScopedID != "" && identityB.TurnScopedID != "" {
		return identityA.TurnID == identityB.TurnID && identityA.TurnScopedID == identityB.TurnScopedID
	}
	return false
}

func (p defaultTranscriptIdentityPolicy) CanFinalizeReplace(current, candidate TranscriptIdentityBlock) bool {
	return p.Equivalent(current, candidate)
}

func identityRolesCompatible(left, right string) bool {
	left = strings.ToLower(strings.TrimSpace(left))
	right = strings.ToLower(strings.TrimSpace(right))
	if left == "" || right == "" {
		return true
	}
	return left == right
}

func firstIdentityValue(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func identityMetaString(meta map[string]any, keys ...string) string {
	if len(meta) == 0 {
		return ""
	}
	for _, key := range keys {
		if value := strings.TrimSpace(identityAnyString(meta[key])); value != "" {
			return value
		}
	}
	return ""
}

func identityAnyString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	case interface{ String() string }:
		return typed.String()
	default:
		return ""
	}
}
