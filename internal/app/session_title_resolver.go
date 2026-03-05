package app

import (
	"strings"

	"control/internal/types"
)

func ResolveSessionTitle(session *types.Session, meta *types.SessionMeta, fallbackID string) string {
	if meta != nil {
		if title := strings.TrimSpace(meta.Title); title != "" {
			return cleanTitle(title)
		}
		if initial := strings.TrimSpace(meta.InitialInput); initial != "" {
			return cleanTitle(initial)
		}
	}
	if session != nil {
		if title := strings.TrimSpace(session.Title); title != "" {
			return cleanTitle(title)
		}
		if id := strings.TrimSpace(session.ID); id != "" {
			return id
		}
	}
	if id := strings.TrimSpace(fallbackID); id != "" {
		return id
	}
	return ""
}
