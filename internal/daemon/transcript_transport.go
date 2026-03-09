package daemon

import (
	"context"
	"errors"
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

var ErrTranscriptFollowUnavailable = errors.New("transcript follow unavailable")

type transcriptTransportSelection struct {
	transport       transcriptTransport
	followAvailable bool
}

type TranscriptTransportSelector interface {
	Select(ctx context.Context, sessionID, provider string) (transcriptTransportSelection, error)
}

type transcriptEventsSubscriber func(ctx context.Context, id string) (<-chan types.CodexEvent, func(), error)
type transcriptItemsSubscriber func(ctx context.Context, id string) (<-chan map[string]any, func(), error)

type defaultTranscriptTransportSelector struct {
	subscribeEvents transcriptEventsSubscriber
	subscribeItems  transcriptItemsSubscriber
}

type selectorTranscriptIngressFactory struct {
	selector TranscriptTransportSelector
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

func NewSelectorTranscriptIngressFactory(selector TranscriptTransportSelector) TranscriptIngressFactory {
	return &selectorTranscriptIngressFactory{selector: selector}
}

func (f *selectorTranscriptIngressFactory) Open(ctx context.Context, sessionID, provider string) (TranscriptIngressHandle, error) {
	if f == nil || f.selector == nil {
		return TranscriptIngressHandle{}, unavailableError("transcript ingress factory unavailable", nil)
	}
	selection, err := f.selector.Select(ctx, sessionID, provider)
	if err != nil {
		return TranscriptIngressHandle{}, err
	}
	closeFn := func() {}
	if selection.transport.eventsCancel != nil || selection.transport.itemsCancel != nil {
		closeFn = func() {
			if selection.transport.eventsCancel != nil {
				selection.transport.eventsCancel()
			}
			if selection.transport.itemsCancel != nil {
				selection.transport.itemsCancel()
			}
		}
	}
	return TranscriptIngressHandle{
		Events:          selection.transport.eventsCh,
		Items:           selection.transport.itemsCh,
		FollowAvailable: selection.followAvailable,
		Close:           closeFn,
	}, nil
}

func (s *defaultTranscriptTransportSelector) Select(ctx context.Context, sessionID, provider string) (transcriptTransportSelection, error) {
	provider = normalizeTranscriptProvider(provider)
	sessionID = strings.TrimSpace(sessionID)
	caps := providers.CapabilitiesFor(provider)
	transport := transcriptTransport{}
	var err error

	if caps.SupportsEvents && s.subscribeEvents != nil {
		transport.eventsCh, transport.eventsCancel, err = s.subscribeEvents(ctx, sessionID)
		if err != nil && !caps.UsesItems {
			if errors.Is(err, ErrTranscriptFollowUnavailable) {
				return transcriptTransportSelection{followAvailable: false}, nil
			}
			return transcriptTransportSelection{}, err
		}
		if err != nil {
			transport.eventsCh = nil
			transport.eventsCancel = nil
		}
	}
	if caps.UsesItems && transport.eventsCh == nil && s.subscribeItems != nil {
		transport.itemsCh, transport.itemsCancel, err = s.subscribeItems(ctx, sessionID)
		if err != nil {
			if errors.Is(err, ErrTranscriptFollowUnavailable) {
				return transcriptTransportSelection{followAvailable: false}, nil
			}
			if transport.eventsCancel == nil {
				return transcriptTransportSelection{}, err
			}
		}
	}
	if transport.eventsCh == nil && transport.itemsCh == nil {
		return transcriptTransportSelection{}, invalidError("provider does not support transcript streaming", nil)
	}
	return transcriptTransportSelection{
		transport:       transport,
		followAvailable: true,
	}, nil
}
