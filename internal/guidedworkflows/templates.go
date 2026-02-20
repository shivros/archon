package guidedworkflows

func BuiltinTemplateSolidPhaseDelivery() WorkflowTemplate {
	tpl, ok := defaultWorkflowTemplateByID(TemplateIDSolidPhaseDelivery)
	if !ok {
		panic("guidedworkflows: default template solid_phase_delivery is missing")
	}
	return tpl
}

func BuiltinTemplates() map[string]WorkflowTemplate {
	templates := DefaultWorkflowTemplates()
	out := make(map[string]WorkflowTemplate, len(templates))
	for _, tpl := range templates {
		id := tpl.ID
		if id == "" {
			continue
		}
		out[id] = cloneTemplate(tpl)
	}
	return out
}
