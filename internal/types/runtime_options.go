package types

import "strings"

type ReasoningLevel string

const (
	ReasoningLow       ReasoningLevel = "low"
	ReasoningMedium    ReasoningLevel = "medium"
	ReasoningHigh      ReasoningLevel = "high"
	ReasoningExtraHigh ReasoningLevel = "extra_high"
)

type AccessLevel string

const (
	AccessReadOnly  AccessLevel = "read_only"
	AccessOnRequest AccessLevel = "on_request"
	AccessFull      AccessLevel = "full_access"
)

func NormalizeReasoningLevel(raw ReasoningLevel) (ReasoningLevel, bool) {
	value := strings.ToLower(strings.TrimSpace(string(raw)))
	value = strings.ReplaceAll(value, "-", "_")
	switch ReasoningLevel(value) {
	case ReasoningLow, ReasoningMedium, ReasoningHigh, ReasoningExtraHigh:
		return ReasoningLevel(value), true
	default:
		return "", false
	}
}

func NormalizeAccessLevel(raw AccessLevel) (AccessLevel, bool) {
	value := strings.ToLower(strings.TrimSpace(string(raw)))
	value = strings.ReplaceAll(value, "-", "_")
	switch value {
	case "":
		return "", true
	case "read_only", "readonly":
		return AccessReadOnly, true
	case "on_request", "onrequest":
		return AccessOnRequest, true
	case "full_access", "fullaccess":
		return AccessFull, true
	default:
		return "", false
	}
}

type SessionRuntimeOptions struct {
	Model     string         `json:"model,omitempty"`
	Reasoning ReasoningLevel `json:"reasoning,omitempty"`
	Access    AccessLevel    `json:"access,omitempty"`
	Provider  map[string]any `json:"provider,omitempty"`
	Version   int            `json:"version,omitempty"`
}

type ProviderOptionCatalog struct {
	Provider              string                      `json:"provider"`
	Models                []string                    `json:"models,omitempty"`
	ReasoningLevels       []ReasoningLevel            `json:"reasoning_levels,omitempty"`
	ModelReasoningLevels  map[string][]ReasoningLevel `json:"model_reasoning_levels,omitempty"`
	ModelDefaultReasoning map[string]ReasoningLevel   `json:"model_default_reasoning,omitempty"`
	AccessLevels          []AccessLevel               `json:"access_levels,omitempty"`
	Defaults              SessionRuntimeOptions       `json:"defaults"`
}

func CloneRuntimeOptions(in *SessionRuntimeOptions) *SessionRuntimeOptions {
	if in == nil {
		return nil
	}
	out := *in
	if in.Provider != nil {
		out.Provider = map[string]any{}
		for key, value := range in.Provider {
			out.Provider[key] = value
		}
	}
	return &out
}

func MergeRuntimeOptions(base *SessionRuntimeOptions, patch *SessionRuntimeOptions) *SessionRuntimeOptions {
	if base == nil && patch == nil {
		return nil
	}
	out := CloneRuntimeOptions(base)
	if out == nil {
		out = &SessionRuntimeOptions{}
	}
	if patch == nil {
		return out
	}
	if model := strings.TrimSpace(patch.Model); model != "" {
		out.Model = model
	}
	if patch.Reasoning != "" {
		out.Reasoning = patch.Reasoning
	}
	if patch.Access != "" {
		out.Access = patch.Access
	}
	if patch.Version != 0 {
		out.Version = patch.Version
	}
	if patch.Provider != nil {
		if out.Provider == nil {
			out.Provider = map[string]any{}
		}
		for key, value := range patch.Provider {
			out.Provider[key] = value
		}
	}
	return out
}
