package daemon

import (
	"context"
	"strings"
	"time"
)

const defaultTurnActivityProbeTimeout = 750 * time.Millisecond

type turnActivityStatus uint8

const (
	turnActivityUnknown turnActivityStatus = iota
	turnActivityActive
	turnActivityInactive
)

type turnActivityProbe interface {
	Probe(ctx context.Context, reader codexTurnReader, threadID, turnID string) (turnActivityStatus, error)
}

type codexTurnReader interface {
	ReadThread(ctx context.Context, threadID string) (*codexThread, error)
}

// codexThreadTurnActivityProbe inspects provider thread state for a turn status.
type codexThreadTurnActivityProbe struct {
	timeout time.Duration
}

func (p codexThreadTurnActivityProbe) Probe(
	ctx context.Context,
	reader codexTurnReader,
	threadID string,
	turnID string,
) (turnActivityStatus, error) {
	if reader == nil {
		return turnActivityUnknown, nil
	}
	threadID = strings.TrimSpace(threadID)
	turnID = strings.TrimSpace(turnID)
	if threadID == "" || turnID == "" {
		return turnActivityUnknown, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	timeout := p.timeout
	if timeout <= 0 {
		timeout = defaultTurnActivityProbeTimeout
	}
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	thread, err := reader.ReadThread(probeCtx, threadID)
	if err != nil {
		return turnActivityUnknown, err
	}
	if thread == nil {
		return turnActivityUnknown, nil
	}
	for _, turn := range thread.Turns {
		if strings.TrimSpace(turn.ID) != turnID {
			continue
		}
		status := normalizeTurnState(turn.Status)
		if status == "" {
			return turnActivityUnknown, nil
		}
		if isTerminalTurnState(status) {
			return turnActivityInactive, nil
		}
		return turnActivityActive, nil
	}
	return turnActivityUnknown, nil
}

func normalizeTurnState(state string) string {
	return strings.ToLower(strings.TrimSpace(state))
}

func isTerminalTurnState(state string) bool {
	switch normalizeTurnState(state) {
	case "completed", "complete", "done", "failed", "error", "interrupted", "cancelled", "canceled", "rejected", "aborted", "stopped":
		return true
	default:
		return false
	}
}
