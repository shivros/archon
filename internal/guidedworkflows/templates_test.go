package guidedworkflows

import "testing"

func TestBuiltinTemplateSolidPhaseDeliverySequence(t *testing.T) {
	tpl := BuiltinTemplateSolidPhaseDelivery()
	if tpl.ID != TemplateIDSolidPhaseDelivery {
		t.Fatalf("unexpected template id: %q", tpl.ID)
	}
	if tpl.Name != "SOLID Phase Delivery" {
		t.Fatalf("unexpected template name: %q", tpl.Name)
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
}
