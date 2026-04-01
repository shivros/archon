package guidedworkflows

import (
	"bytes"
	"encoding/json"
	"strings"
)

type directWorkflowTemplateCatalog struct {
	Version   int                `json:"version"`
	Templates []WorkflowTemplate `json:"templates"`
}

type workflowTemplateCatalogProbe struct {
	Definitions json.RawMessage   `json:"definitions,omitempty"`
	Templates   []json.RawMessage `json:"templates"`
}

type workflowTemplateProbe struct {
	Phases []json.RawMessage `json:"phases,omitempty"`
}

type workflowTemplatePhaseProbe struct {
	Gates []json.RawMessage `json:"gates,omitempty"`
}

func DecodeWorkflowTemplateCatalogJSON(raw []byte) (ParsedWorkflowTemplateCatalog, error) {
	parsed, err := ParseWorkflowTemplateCatalogJSON(raw)
	if err == nil {
		return parsed, nil
	}

	if !catalogUsesTypedTemplateEncoding(raw) {
		return ParsedWorkflowTemplateCatalog{}, err
	}

	return decodeDirectWorkflowTemplateCatalogJSON(raw)
}

func decodeDirectWorkflowTemplateCatalogJSON(raw []byte) (ParsedWorkflowTemplateCatalog, error) {
	var file directWorkflowTemplateCatalog
	if err := json.Unmarshal(raw, &file); err != nil {
		return ParsedWorkflowTemplateCatalog{}, err
	}
	out := ParsedWorkflowTemplateCatalog{
		Version:   file.Version,
		Templates: make([]WorkflowTemplate, 0, len(file.Templates)),
	}
	for _, template := range file.Templates {
		normalized, err := NormalizeWorkflowTemplate(template)
		if err != nil {
			return ParsedWorkflowTemplateCatalog{}, err
		}
		out.Templates = append(out.Templates, normalized)
	}
	return out, nil
}

func catalogUsesTypedTemplateEncoding(raw []byte) bool {
	var probe workflowTemplateCatalogProbe
	if err := json.Unmarshal(raw, &probe); err != nil {
		return false
	}
	if len(bytes.TrimSpace(probe.Definitions)) > 0 && !bytes.Equal(bytes.TrimSpace(probe.Definitions), []byte("null")) {
		return false
	}
	for _, templateRaw := range probe.Templates {
		if templateUsesTypedTemplateEncoding(templateRaw) {
			return true
		}
	}
	return false
}

func templateUsesTypedTemplateEncoding(raw []byte) bool {
	var probe workflowTemplateProbe
	if err := json.Unmarshal(raw, &probe); err != nil {
		return false
	}
	for _, phaseRaw := range probe.Phases {
		if phaseUsesTypedTemplateEncoding(phaseRaw) {
			return true
		}
	}
	return false
}

func phaseUsesTypedTemplateEncoding(raw []byte) bool {
	var probe workflowTemplatePhaseProbe
	if err := json.Unmarshal(raw, &probe); err != nil {
		return false
	}
	for _, gateRaw := range probe.Gates {
		if gateUsesTypedTemplateEncoding(gateRaw) {
			return true
		}
	}
	return false
}

func gateUsesTypedTemplateEncoding(raw []byte) bool {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return false
	}
	for name, value := range fields {
		if strings.HasSuffix(name, "_config") {
			return true
		}
		if name == "boundary" {
			trimmed := bytes.TrimSpace(value)
			if len(trimmed) > 0 && trimmed[0] == '{' {
				return true
			}
		}
	}
	return false
}
