package daemon

import "control/internal/types"

type debugEventStore interface {
	Append(event types.DebugEvent)
}

type debugEventBus interface {
	Broadcast(event types.DebugEvent)
}

type debugChunkSink interface {
	Write(stream string, data []byte)
	Close()
}
