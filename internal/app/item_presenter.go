package app

import "time"

type TranscriptItemPresenter interface {
	Present(item map[string]any, createdAt time.Time, now time.Time) (ChatBlock, bool)
}

type defaultTranscriptItemPresenter struct {
	providerPolicy ProviderDisplayPolicy
}

func NewDefaultTranscriptItemPresenter(providerPolicy ProviderDisplayPolicy) TranscriptItemPresenter {
	if providerPolicy == nil {
		providerPolicy = DefaultProviderDisplayPolicy()
	}
	return defaultTranscriptItemPresenter{providerPolicy: providerPolicy}
}

func (p defaultTranscriptItemPresenter) Present(item map[string]any, createdAt time.Time, now time.Time) (ChatBlock, bool) {
	if item == nil {
		return ChatBlock{}, false
	}
	if text, ok := presentRateLimitSystemText(item, now, p.providerPolicy); ok && text != "" {
		return ChatBlock{
			Role:      ChatRoleSystem,
			Text:      text,
			Status:    ChatStatusNone,
			CreatedAt: createdAt,
		}, true
	}
	return ChatBlock{}, false
}
