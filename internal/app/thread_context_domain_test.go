package app

import "testing"

type threadContextTestAdapter struct {
	provider string
	metrics  ThreadContextMetrics
}

func (a threadContextTestAdapter) ProviderName() string {
	return a.provider
}

func (a threadContextTestAdapter) Metrics(ThreadContextMetricsInput) ThreadContextMetrics {
	return a.metrics
}

func TestNewDefaultThreadContextMetricsServiceAdapterNormalizationAndOverride(t *testing.T) {
	one := int64(1)
	two := int64(2)
	service := NewDefaultThreadContextMetricsService([]ThreadContextProviderMetricsAdapter{
		nil,
		threadContextTestAdapter{provider: "   ", metrics: ThreadContextMetrics{Tokens: &one}},
		threadContextTestAdapter{provider: " Codex ", metrics: ThreadContextMetrics{Tokens: &one}},
		threadContextTestAdapter{provider: "codex", metrics: ThreadContextMetrics{Tokens: &two}},
	})

	data := service.BuildPanelData(ThreadContextMetricsInput{Provider: "CODEX"})
	if data.Metrics.Tokens == nil || *data.Metrics.Tokens != 2 {
		t.Fatalf("expected last normalized adapter to win, got %#v", data.Metrics)
	}
}

func TestThreadContextMetricsServiceBuildPanelDataAdapterMissAndTitleFallback(t *testing.T) {
	service := NewDefaultThreadContextMetricsService(nil)
	data := service.BuildPanelData(ThreadContextMetricsInput{SessionID: "s-1"})
	if data.ThreadTitle != "s-1" {
		t.Fatalf("expected session id title fallback, got %q", data.ThreadTitle)
	}
	if data.Metrics.Tokens != nil || data.Metrics.ContextUsedPct != nil || data.Metrics.SpendUSD != nil {
		t.Fatalf("expected nil metrics on adapter miss, got %#v", data.Metrics)
	}
}

func TestWithThreadContextMetricsServiceNilResetsToDefault(t *testing.T) {
	provided := int64(123)
	m := NewModel(nil,
		WithThreadContextMetricsService(stubThreadContextMetricsService{data: ThreadContextPanelData{Metrics: ThreadContextMetrics{Tokens: &provided}}}),
		WithThreadContextMetricsService(nil),
	)
	data := m.threadContextMetricsServiceOrDefault().BuildPanelData(ThreadContextMetricsInput{Provider: "codex"})
	if data.Metrics.Tokens != nil {
		t.Fatalf("expected reset to default service, got %#v", data.Metrics)
	}
}

func TestThreadContextMetricsServiceOrDefaultNilModel(t *testing.T) {
	var m *Model
	service := m.threadContextMetricsServiceOrDefault()
	if service == nil {
		t.Fatalf("expected default service for nil model")
	}
	data := service.BuildPanelData(ThreadContextMetricsInput{SessionID: "s1"})
	if data.ThreadTitle != "s1" {
		t.Fatalf("expected fallback title from default service, got %q", data.ThreadTitle)
	}
}

func TestWithThreadContextMetricsServiceOptionNilModelNoPanic(t *testing.T) {
	opt := WithThreadContextMetricsService(nil)
	var m *Model
	opt(m)
}
