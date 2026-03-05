package app

import (
	"strings"

	"control/internal/types"
)

type ThreadContextMetrics struct {
	Tokens         *int64
	ContextUsedPct *float64
	SpendUSD       *float64
}

type ThreadContextPanelData struct {
	ThreadTitle string
	Metrics     ThreadContextMetrics
}

type ThreadContextMetricsInput struct {
	Provider    string
	SessionID   string
	Session     *types.Session
	SessionMeta *types.SessionMeta
}

type ThreadContextProviderMetricsAdapter interface {
	ProviderName() string
	Metrics(ThreadContextMetricsInput) ThreadContextMetrics
}

type ThreadContextMetricsService interface {
	BuildPanelData(ThreadContextMetricsInput) ThreadContextPanelData
}

type defaultThreadContextMetricsService struct {
	adapters map[string]ThreadContextProviderMetricsAdapter
}

func NewDefaultThreadContextMetricsService(adapters []ThreadContextProviderMetricsAdapter) ThreadContextMetricsService {
	index := map[string]ThreadContextProviderMetricsAdapter{}
	for _, adapter := range adapters {
		if adapter == nil {
			continue
		}
		provider := strings.ToLower(strings.TrimSpace(adapter.ProviderName()))
		if provider == "" {
			continue
		}
		index[provider] = adapter
	}
	return defaultThreadContextMetricsService{adapters: index}
}

func WithThreadContextMetricsService(service ThreadContextMetricsService) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if service == nil {
			m.threadContextMetricsService = NewDefaultThreadContextMetricsService(nil)
			return
		}
		m.threadContextMetricsService = service
	}
}

func (m *Model) threadContextMetricsServiceOrDefault() ThreadContextMetricsService {
	if m == nil || m.threadContextMetricsService == nil {
		return NewDefaultThreadContextMetricsService(nil)
	}
	return m.threadContextMetricsService
}

func (s defaultThreadContextMetricsService) BuildPanelData(input ThreadContextMetricsInput) ThreadContextPanelData {
	title := ResolveSessionTitle(input.Session, input.SessionMeta, input.SessionID)
	metrics := ThreadContextMetrics{}
	provider := strings.ToLower(strings.TrimSpace(input.Provider))
	if provider != "" {
		if adapter, ok := s.adapters[provider]; ok && adapter != nil {
			metrics = adapter.Metrics(input)
		}
	}
	return ThreadContextPanelData{ThreadTitle: title, Metrics: metrics}
}
