package guidedworkflows

func CloneWorkflowTemplate(in WorkflowTemplate) WorkflowTemplate {
	return cloneTemplate(in)
}

func CloneWorkflowTemplates(in []WorkflowTemplate) []WorkflowTemplate {
	out := make([]WorkflowTemplate, 0, len(in))
	for _, template := range in {
		out = append(out, CloneWorkflowTemplate(template))
	}
	return out
}
