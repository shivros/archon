package daemon

import (
	"context"
	"strings"

	"control/internal/providers"
	"control/internal/types"
)

type transcriptTransport struct {
	eventsCh     <-chan types.CodexEvent
	eventsCancel func()
	itemsCh      <-chan map[string]any
	itemsCancel  func()
}

type TranscriptTransportSelector interface {
	Select(ctx context.Context, sessionID, provider string) (transcriptTransport, error)
}

type transcriptEventsSubscriber func(ctx context.Context, id string) (<-chan types.CodexEvent, func(), error)
type transcriptItemsSubscriber func(ctx context.Context, id string) (<-chan map[string]any, func(), error)

type defaultTranscriptTransportSelector struct {
	subscribeEvents transcriptEventsSubscriber
	subscribeItems  transcriptItemsSubscriber
}

func NewDefaultTranscriptTransportSelector(
	subscribeEvents transcriptEventsSubscriber,
	subscribeItems transcriptItemsSubscriber,
) TranscriptTransportSelector {
	return &defaultTranscriptTransportSelector{
		subscribeEvents: subscribeEvents,
		subscribeItems:  subscribeItems,
	}
}

func (s *defaultTranscriptTransportSelector) Select(ctx context.Context, sessionID, provider string) (transcriptTransport, error) {
	provider = normalizeTranscriptProvider(provider)
	sessionID = strings.TrimSpace(sessionID)
	caps := providers.CapabilitiesFor(provider)
	transport := transcriptTransport{}
	var err error

	if caps.SupportsEvents && s.subscribeEvents != nil {
		transport.eventsCh, transport.eventsCancel, err = s.subscribeEvents(ctx, sessionID)
		if err != nil && !caps.UsesItems {
			return transcriptTransport{}, err
		}
		if err != nil {
			transport.eventsCh = nil
			transport.eventsCancel = nil
		}
	}
	if caps.UsesItems && transport.eventsCh == nil && s.subscribeItems != nil {
		transport.itemsCh, transport.itemsCancel, err = s.subscribeItems(ctx, sessionID)
		if err != nil {
			if transport.eventsCancel == nil {
				return transcriptTransport{}, err
			}
		}
	}
	if transport.eventsCh == nil && transport.itemsCh == nil {
		return transcriptTransport{}, invalidError("provider does not support transcript streaming", nil)
	}
	return transport, nil
}
