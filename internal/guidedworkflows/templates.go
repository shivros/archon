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
					{
						ID:     "phase_plan",
						Name:   "phase plan",
						Prompt: "Please write up an end-to-end, SOLID-compliant plan for this work; a plan I can give to my AI CLI LLM agent locally to implement? I want to be able to paste it in phases - 1 at a time so I can review the work incrementally. And make the instructions somewhat flexible, since the local agent has a lot more context and may need to make pragmatic adjustments.",
					},
					{
						ID:     "implementation",
						Name:   "implementation",
						Prompt: "Sounds good. Please proceed with implementation.",
					},
					{
						ID:     "solid_audit",
						Name:   "SOLID audit",
						Prompt: "Please audit your work for compliance with SOLID principles.",
					},
					{
						ID:     "mitigation_plan",
						Name:   "mitigation plan",
						Prompt: "Please write a SOLID-compliant plan for addressing the issues you called out",
					},
					{
						ID:     "mitigation_implementation",
						Name:   "mitigation implementation",
						Prompt: "Sounds good. Please proceed with implementation.",
					},
					{
						ID:     "test_gap_audit",
						Name:   "test gap audit",
						Prompt: "Please audit your work for gaps in test coverage. If no coverage tooling is available, check manually.",
					},
					{
						ID:     "test_implementation",
						Name:   "test implementation",
						Prompt: "Sounds good. Please proceed with implementation.",
					},
					{
						ID:     "quality_checks",
						Name:   "quality checks",
						Prompt: "Please run all relevant tests and quality checks. Run them iteratively, attempt to fix any issues that are fixable, then run the commands again to verify they are fixed. If you face any issues that are not reasonably fixable, document them and let me know.",
					},
					{
						ID:     "commit",
						Name:   "commit",
						Prompt: "Please commit your work. Please keep your commits compliant with Conventional Commit structure (https://www.conventionalcommits.org/en/v1.0.0/)",
					},
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
