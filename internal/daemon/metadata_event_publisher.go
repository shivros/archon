package daemon

import "control/internal/types"

type MetadataEventPublisher interface {
	PublishMetadataEvent(event types.MetadataEvent)
}
