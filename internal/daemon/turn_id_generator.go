package daemon

import (
	"fmt"
	"strings"

	"control/internal/logging"
	"control/internal/providers"
)

type turnIDGenerator interface {
	NewTurnID(provider string) string
}

type defaultTurnIDGenerator struct{}

func (defaultTurnIDGenerator) NewTurnID(provider string) string {
	provider = providers.Normalize(provider)
	if provider == "" {
		provider = "provider"
	}
	randomID := strings.TrimSpace(logging.NewRequestID())
	if randomID == "" {
		randomID = "turn"
	}
	return fmt.Sprintf("%s-turn-%s", provider, randomID)
}
