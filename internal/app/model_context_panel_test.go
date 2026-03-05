package app

import (
	"strings"
	"testing"
	"time"

	xansi "github.com/charmbracelet/x/ansi"

	"control/internal/types"
)

type stubSidePanelModePolicy struct {
	mode sidePanelMode
}

func (p stubSidePanelModePolicy) Resolve(*Model) sidePanelMode {
	return p.mode
}

type stubThreadContextMetricsService struct {
	data ThreadContextPanelData
}

func (s stubThreadContextMetricsService) BuildPanelData(ThreadContextMetricsInput) ThreadContextPanelData {
	return s.data
}

func TestActiveSidePanelModeUsesContextInComposeMode(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeCompose
	m.notesPanelOpen = true
	m.resize(180, 40)

	if got := m.activeSidePanelMode(); got != sidePanelModeContext {
		t.Fatalf("expected side panel mode context, got %v", got)
	}
	if !m.contextPanelVisible {
		t.Fatalf("expected context panel to be visible")
	}
	if m.notesPanelVisible {
		t.Fatalf("expected notes panel to be hidden in compose mode")
	}
}

func TestActiveSidePanelModePrefersDebugOverContext(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeCompose
	m.appState.DebugStreamsEnabled = true
	m.resize(180, 40)

	if got := m.activeSidePanelMode(); got != sidePanelModeDebug {
		t.Fatalf("expected side panel mode debug, got %v", got)
	}
	if !m.debugPanelVisible {
		t.Fatalf("expected debug panel to be visible")
	}
	if m.contextPanelVisible {
		t.Fatalf("expected context panel to be hidden when debug is enabled")
	}
}

func TestRenderContextPanelViewFormatsHeaderAndFallbackMetrics(t *testing.T) {
	m := NewModel(nil)
	now := time.Now().UTC()
	m.sessions = []*types.Session{{ID: "s1", Provider: "codex", CreatedAt: now}}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", Title: "Refactor API"},
	}
	if m.compose != nil {
		m.compose.SetSession("s1", "Refactor API")
	}

	text := xansi.Strip(m.renderContextPanelView())
	if !strings.Contains(text, "Refactor API") {
		t.Fatalf("expected thread title in panel, got %q", text)
	}
	if !strings.Contains(text, "Context") {
		t.Fatalf("expected context header in panel, got %q", text)
	}
	if !strings.Contains(text, "\n-\n-\n-") {
		t.Fatalf("expected unsupported metrics to render as dashes, got %q", text)
	}
}

func TestFormatThreadContextValues(t *testing.T) {
	tokens := int64(1123312)
	if got := formatTokensOrDash(&tokens); got != "1,123,312 tokens" {
		t.Fatalf("unexpected tokens format: %q", got)
	}
	pct := 12.4
	if got := formatContextUsedOrDash(&pct); got != "12% used" {
		t.Fatalf("unexpected context format: %q", got)
	}
	spend := 12.12
	if got := formatSpendOrDash(&spend); got != "$12.12 spend" {
		t.Fatalf("unexpected spend format: %q", got)
	}
}

func TestFormatIntWithCommasNegative(t *testing.T) {
	if got := formatIntWithCommas(-1234567); got != "-1,234,567" {
		t.Fatalf("unexpected negative comma format: %q", got)
	}
}

func TestWithSidePanelModePolicyOverridesResolution(t *testing.T) {
	m := NewModel(nil, WithSidePanelModePolicy(stubSidePanelModePolicy{mode: sidePanelModeNotes}))
	if got := m.activeSidePanelMode(); got != sidePanelModeNotes {
		t.Fatalf("expected overridden mode %v, got %v", sidePanelModeNotes, got)
	}
}

func TestWithThreadContextMetricsServiceOverridesPanelData(t *testing.T) {
	service := stubThreadContextMetricsService{
		data: ThreadContextPanelData{
			ThreadTitle: "Injected",
			Metrics: ThreadContextMetrics{
				Tokens: func() *int64 { v := int64(123); return &v }(),
			},
		},
	}
	m := NewModel(nil, WithThreadContextMetricsService(service))
	text := xansi.Strip(m.renderContextPanelView())
	if !strings.Contains(text, "Injected") {
		t.Fatalf("expected injected title, got %q", text)
	}
	if !strings.Contains(text, "123 tokens") {
		t.Fatalf("expected injected token metric, got %q", text)
	}
}

func TestRenderContextPanelViewFallsBackToThreadTitle(t *testing.T) {
	m := NewModel(nil, WithThreadContextMetricsService(stubThreadContextMetricsService{
		data: ThreadContextPanelData{ThreadTitle: "   "},
	}))
	text := xansi.Strip(m.renderContextPanelView())
	if !strings.Contains(text, "Thread") {
		t.Fatalf("expected thread fallback title, got %q", text)
	}
}
