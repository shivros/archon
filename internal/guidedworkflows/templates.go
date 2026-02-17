package guidedworkflows

func BuiltinTemplateSolidPhaseDelivery() WorkflowTemplate {
	return WorkflowTemplate{
		ID:          TemplateIDSolidPhaseDelivery,
		Name:        "SOLID Phase Delivery",
		Description: "Phased delivery loop with SOLID-focused audits and mitigation steps.",
		Phases: []WorkflowTemplatePhase{
			{
				ID:   "phase_delivery",
				Name: "Delivery Phase",
				Steps: []WorkflowTemplateStep{
					{ID: "phase_plan", Name: "phase plan"},
					{ID: "implementation", Name: "implementation"},
					{ID: "solid_audit", Name: "SOLID audit"},
					{ID: "mitigation_plan", Name: "mitigation plan"},
					{ID: "mitigation_implementation", Name: "mitigation implementation"},
					{ID: "test_gap_audit", Name: "test gap audit"},
					{ID: "test_implementation", Name: "test implementation"},
					{ID: "quality_checks", Name: "quality checks"},
					{ID: "commit", Name: "commit"},
				},
			},
		},
	}
}

func BuiltinTemplates() map[string]WorkflowTemplate {
	tpl := BuiltinTemplateSolidPhaseDelivery()
	return map[string]WorkflowTemplate{
		tpl.ID: tpl,
	}
}
