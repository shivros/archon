package daemon

import (
	"strconv"
	"testing"
	"time"

	"control/internal/types"
)

func TestMetadataEventHubPublishAndSubscribe(t *testing.T) {
	hub := newMetadataEventHub(nil)
	ch, cancel, err := hub.Subscribe("")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cancel()

	hub.PublishMetadataEvent(types.MetadataEvent{
		Type: types.MetadataEventTypeSessionUpdated,
		Session: &types.MetadataEntityUpdated{
			ID:    "s1",
			Title: "Renamed",
		},
	})

	select {
	case event := <-ch:
		if event.Type != types.MetadataEventTypeSessionUpdated {
			t.Fatalf("unexpected event type: %q", event.Type)
		}
		if event.Revision == "" {
			t.Fatalf("expected revision")
		}
		if event.Session == nil || event.Session.ID != "s1" || event.Session.Title != "Renamed" {
			t.Fatalf("unexpected session payload: %#v", event.Session)
		}
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for event")
	}
}

func TestMetadataEventHubSubscribeAfterRevisionReplaysNewer(t *testing.T) {
	hub := newMetadataEventHub(nil)
	hub.PublishMetadataEvent(types.MetadataEvent{
		Type: types.MetadataEventTypeSessionUpdated,
		Session: &types.MetadataEntityUpdated{
			ID:    "s1",
			Title: "v1",
		},
	})
	hub.PublishMetadataEvent(types.MetadataEvent{
		Type: types.MetadataEventTypeSessionUpdated,
		Session: &types.MetadataEntityUpdated{
			ID:    "s1",
			Title: "v2",
		},
	})

	ch, cancel, err := hub.Subscribe("1")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cancel()
	select {
	case event := <-ch:
		if event.Revision != "2" {
			t.Fatalf("expected replay revision 2, got %q", event.Revision)
		}
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for replay")
	}
}

func TestMetadataEventHubSubscribeInvalidRevision(t *testing.T) {
	hub := newMetadataEventHub(nil)
	if _, _, err := hub.Subscribe("abc"); err == nil {
		t.Fatalf("expected invalid revision error")
	}
}

func TestMetadataEventHubDropOnSlowSubscriber(t *testing.T) {
	hub := newMetadataEventHub(nil)
	ch, cancel, err := hub.Subscribe("")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cancel()
	for i := 0; i < metadataEventSubscriberBuffer+32; i++ {
		hub.PublishMetadataEvent(types.MetadataEvent{
			Type: types.MetadataEventTypeSessionUpdated,
			Session: &types.MetadataEntityUpdated{
				ID:    "s1",
				Title: "t",
			},
		})
	}
	received := 0
drain:
	for {
		select {
		case <-ch:
			received++
		default:
			break drain
		}
	}
	if received > metadataEventSubscriberBuffer {
		t.Fatalf("expected buffered drop behavior, got %d events", received)
	}
}

func TestMetadataEventHubReplayEvictsOldRevisions(t *testing.T) {
	hub := newMetadataEventHub(nil)
	total := metadataEventReplayLimit + 10
	for i := 0; i < total; i++ {
		hub.PublishMetadataEvent(types.MetadataEvent{
			Type: types.MetadataEventTypeSessionUpdated,
			Session: &types.MetadataEntityUpdated{
				ID:    "s1",
				Title: "t",
			},
		})
	}

	ch, cancel, err := hub.Subscribe("1")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cancel()
	select {
	case event := <-ch:
		if event.Revision != "11" {
			t.Fatalf("expected replay to start at evicted boundary revision 11, got %q", event.Revision)
		}
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for replay")
	}
}

func TestMetadataEventHubSubscribeBacklogOverflowDrops(t *testing.T) {
	hub := newMetadataEventHub(nil)
	for i := 0; i < metadataEventReplayLimit; i++ {
		hub.PublishMetadataEvent(types.MetadataEvent{
			Type: types.MetadataEventTypeSessionUpdated,
			Session: &types.MetadataEntityUpdated{
				ID:    "s1",
				Title: strconv.Itoa(i),
			},
		})
	}
	ch, cancel, err := hub.Subscribe("")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if cancel == nil {
		t.Fatalf("expected cancel function")
	}
	cancel()
	cancel()
	if got := len(ch); got != metadataEventSubscriberBuffer {
		t.Fatalf("expected buffered backlog capped at %d, got %d", metadataEventSubscriberBuffer, got)
	}
}

func TestMetadataEventHubNilReceiverSafe(t *testing.T) {
	var hub *metadataEventHub
	ch, cancel, err := hub.Subscribe("")
	if err != nil {
		t.Fatalf("expected nil hub subscribe to succeed, got error %v", err)
	}
	if ch != nil || cancel != nil {
		t.Fatalf("expected nil channel/cancel for nil hub")
	}
	hub.PublishMetadataEvent(types.MetadataEvent{Type: types.MetadataEventTypeSessionUpdated})
}
