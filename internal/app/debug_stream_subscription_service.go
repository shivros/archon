package app

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
)

type DebugStreamSubscriptionContext struct {
	Enabled         bool
	StreamAPIReady  bool
	ConsumerReady   bool
	ActiveSessionID string
	HasStream       bool
	ScopeInFlight   bool
	ScopeName       string
	ReplaceScope    func(name string) context.Context
	Open            func(sessionID string, parent context.Context) tea.Cmd
}

type DebugStreamSubscriptionService interface {
	Ensure(ctx DebugStreamSubscriptionContext) tea.Cmd
}

type defaultDebugStreamSubscriptionService struct{}

func (defaultDebugStreamSubscriptionService) Ensure(ctx DebugStreamSubscriptionContext) tea.Cmd {
	if !ctx.Enabled || !ctx.StreamAPIReady || !ctx.ConsumerReady {
		return nil
	}
	sessionID := strings.TrimSpace(ctx.ActiveSessionID)
	if sessionID == "" || ctx.HasStream || ctx.ScopeInFlight {
		return nil
	}
	if ctx.ReplaceScope == nil || ctx.Open == nil {
		return nil
	}
	scopeName := strings.TrimSpace(ctx.ScopeName)
	parent := ctx.ReplaceScope(scopeName)
	return ctx.Open(sessionID, parent)
}

func WithDebugStreamSubscriptionService(service DebugStreamSubscriptionService) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if service == nil {
			m.debugStreamSubscriptionService = defaultDebugStreamSubscriptionService{}
			return
		}
		m.debugStreamSubscriptionService = service
	}
}

func (m *Model) debugStreamSubscriptionServiceOrDefault() DebugStreamSubscriptionService {
	if m == nil || m.debugStreamSubscriptionService == nil {
		return defaultDebugStreamSubscriptionService{}
	}
	return m.debugStreamSubscriptionService
}
