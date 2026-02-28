package app

import "strings"

type ProviderDisplayPolicy interface {
	DisplayName(provider string) string
}

type defaultProviderDisplayPolicy struct {
	names map[string]string
}

func DefaultProviderDisplayPolicy() ProviderDisplayPolicy {
	return defaultProviderDisplayPolicy{
		names: map[string]string{
			"claude":   "Claude",
			"codex":    "Codex",
			"opencode": "OpenCode",
		},
	}
}

func (p defaultProviderDisplayPolicy) DisplayName(provider string) string {
	raw := strings.TrimSpace(provider)
	if raw == "" {
		return "Provider"
	}
	key := strings.ToLower(raw)
	if label, ok := p.names[key]; ok && strings.TrimSpace(label) != "" {
		return label
	}
	return humanizeProviderName(raw)
}

func humanizeProviderName(provider string) string {
	fields := strings.FieldsFunc(provider, func(r rune) bool {
		return r == '-' || r == '_' || r == '/' || r == ':'
	})
	if len(fields) == 0 {
		return "Provider"
	}
	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		lower := strings.ToLower(field)
		if len(lower) == 1 {
			parts = append(parts, strings.ToUpper(lower))
			continue
		}
		parts = append(parts, strings.ToUpper(lower[:1])+lower[1:])
	}
	if len(parts) == 0 {
		return "Provider"
	}
	return strings.Join(parts, " ")
}
