package guidedworkflows

import (
	"testing"

	"control/internal/types"
)

func TestBuiltinTemplateSolidPhaseDeliverySequence(t *testing.T) {
	tpl := BuiltinTemplateSolidPhaseDelivery()
	if tpl.ID != TemplateIDSolidPhaseDelivery {
		t.Fatalf("unexpected template id: %q", tpl.ID)
	}
	if tpl.Name != "Feature/Bug" {
		t.Fatalf("unexpected template name: %q", tpl.Name)
	}
	if tpl.DefaultAccessLevel != types.AccessFull {
		t.Fatalf("unexpected default access level: %q", tpl.DefaultAccessLevel)
	}
	if len(tpl.Phases) != 1 {
		t.Fatalf("expected one phase, got %d", len(tpl.Phases))
	}
	steps := tpl.Phases[0].Steps
	expected := []string{
		"phase plan",
		"implementation",
		"SOLID audit",
		"mitigation plan",
		"mitigation implementation",
		"test gap audit",
		"test implementation",
		"quality checks",
		"commit",
	}
	if len(steps) != len(expected) {
		t.Fatalf("unexpected steps count: got=%d want=%d", len(steps), len(expected))
	}
	for i, want := range expected {
		if steps[i].Name != want {
			t.Fatalf("unexpected step %d: got=%q want=%q", i, steps[i].Name, want)
		}
		if steps[i].Prompt == "" {
			t.Fatalf("expected step %d (%s) to include default prompt", i, steps[i].ID)
		}
	}
	if steps[0].RuntimeOptions == nil || steps[0].RuntimeOptions.Reasoning != types.ReasoningExtraHigh {
		t.Fatalf("expected phase_plan runtime options override, got %#v", steps[0].RuntimeOptions)
	}
	if steps[1].RuntimeOptions != nil {
		t.Fatalf("expected implementation step to inherit runtime defaults, got %#v", steps[1].RuntimeOptions)
	}
	if steps[3].RuntimeOptions == nil || steps[3].RuntimeOptions.Reasoning != types.ReasoningExtraHigh {
		t.Fatalf("expected mitigation_plan runtime options override, got %#v", steps[3].RuntimeOptions)
	}
	if steps[8].RuntimeOptions == nil || steps[8].RuntimeOptions.Reasoning != types.ReasoningMedium {
		t.Fatalf("expected commit runtime options override, got %#v", steps[8].RuntimeOptions)
	}
}

func TestDefaultWorkflowTemplatesContainsSolidPhaseDelivery(t *testing.T) {
	templates := DefaultWorkflowTemplates()
	if len(templates) != 6 {
		t.Fatalf("expected six default workflow templates, got %d", len(templates))
	}
	found := map[string]bool{}
	for _, tpl := range templates {
		found[tpl.ID] = true
	}
	expectedIDs := []string{
		TemplateIDSolidPhaseDelivery,
		"test_flow",
		"solid_feature_bug_playwright",
		"implement_plan",
		"implement_plan_playwright",
		"implement_simple_task",
	}
	for _, id := range expectedIDs {
		if !found[id] {
			t.Fatalf("expected %q template in defaults", id)
		}
	}
}
