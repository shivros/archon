package app

import "testing"

func TestDefaultDebugPanelInteractionServiceHitTest(t *testing.T) {
	svc := NewDefaultDebugPanelInteractionService()
	spans := []renderedBlockSpan{{
		ID:        "debug-1",
		StartLine: 0,
		EndLine:   4,
		MetaControls: []renderedMetaControlHit{{
			ID:    debugMetaControlCopy,
			Label: "[Copy]",
			Line:  1,
			Start: 5,
			End:   10,
		}},
	}}
	hit, ok := svc.HitTest(spans, 0, 7, 1)
	if !ok {
		t.Fatalf("expected hit test to succeed")
	}
	if hit.BlockID != "debug-1" || hit.ControlID != debugMetaControlCopy {
		t.Fatalf("unexpected hit: %#v", hit)
	}
}

func TestDefaultDebugPanelInteractionServiceGuardBranches(t *testing.T) {
	svc := NewDefaultDebugPanelInteractionService()
	if _, ok := svc.HitTest(nil, 0, 0, 0); ok {
		t.Fatalf("expected no hit for empty spans")
	}
	if _, ok := svc.HitTest([]renderedBlockSpan{{ID: "debug-1", StartLine: 0, EndLine: 0}}, 0, -1, 0); ok {
		t.Fatalf("expected no hit for negative column")
	}
	if _, ok := svc.HitTest([]renderedBlockSpan{{ID: "debug-1", StartLine: 0, EndLine: 0}}, 0, 0, -1); ok {
		t.Fatalf("expected no hit for negative line")
	}
}

func TestDefaultDebugPanelInteractionServiceFallsBackToLabel(t *testing.T) {
	svc := NewDefaultDebugPanelInteractionService()
	spans := []renderedBlockSpan{{
		ID:        "debug-1",
		StartLine: 0,
		EndLine:   2,
		MetaControls: []renderedMetaControlHit{{
			ID:    "",
			Label: "[Expand]",
			Line:  1,
			Start: 3,
			End:   10,
		}},
	}}
	hit, ok := svc.HitTest(spans, 0, 4, 1)
	if !ok {
		t.Fatalf("expected label fallback to resolve control")
	}
	if hit.ControlID != debugMetaControlToggle {
		t.Fatalf("expected toggle control from label fallback, got %q", hit.ControlID)
	}
}

func TestDefaultDebugPanelInteractionServiceUnknownLabelDoesNotHit(t *testing.T) {
	svc := NewDefaultDebugPanelInteractionService()
	spans := []renderedBlockSpan{{
		ID:        "debug-1",
		StartLine: 0,
		EndLine:   2,
		MetaControls: []renderedMetaControlHit{{
			ID:    "",
			Label: "[Unknown]",
			Line:  1,
			Start: 3,
			End:   10,
		}},
	}}
	if _, ok := svc.HitTest(spans, 0, 4, 1); ok {
		t.Fatalf("expected unknown label to return no hit")
	}
}
