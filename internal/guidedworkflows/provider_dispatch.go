package guidedworkflows

import (
	"control/internal/providers"
)

type DispatchProviderPolicy interface {
	Normalize(provider string) string
	SupportsDispatch(provider string) bool
	Validate(provider string) error
}

type DispatchProviderValidationError struct {
	Provider string
}

func (e *DispatchProviderValidationError) Error() string {
	if e == nil {
		return "provider is not dispatchable for guided workflows"
	}
	return "provider is not dispatchable for guided workflows: " + `"` + e.Provider + `"`
}

func (e *DispatchProviderValidationError) Is(target error) bool {
	return target == ErrUnsupportedProvider
}

func UnsupportedDispatchProvider(err error) (string, bool) {
	if err == nil {
		return "", false
	}
	typed, ok := err.(*DispatchProviderValidationError)
	if !ok || typed == nil {
		return "", false
	}
	return typed.Provider, true
}

type registryDispatchProviderPolicy struct{}

func (registryDispatchProviderPolicy) Normalize(provider string) string {
	return providers.Normalize(provider)
}

func (p registryDispatchProviderPolicy) SupportsDispatch(provider string) bool {
	normalized := p.Normalize(provider)
	if normalized == "" {
		return false
	}
	def, ok := providers.Lookup(normalized)
	if !ok {
		return false
	}
	return CanDispatchGuidedWorkflow(def)
}

func (p registryDispatchProviderPolicy) Validate(provider string) error {
	normalized := p.Normalize(provider)
	if normalized == "" {
		return nil
	}
	if p.SupportsDispatch(normalized) {
		return nil
	}
	return &DispatchProviderValidationError{Provider: normalized}
}

var defaultDispatchProviderPolicy DispatchProviderPolicy = registryDispatchProviderPolicy{}

func DefaultDispatchProviderPolicy() DispatchProviderPolicy {
	return defaultDispatchProviderPolicy
}

func NormalizeDispatchProvider(provider string) string {
	return DefaultDispatchProviderPolicy().Normalize(provider)
}

func SupportsDispatchProvider(provider string) bool {
	return DefaultDispatchProviderPolicy().SupportsDispatch(provider)
}

func ValidateDispatchProvider(provider string) error {
	return DefaultDispatchProviderPolicy().Validate(provider)
}

func CanDispatchGuidedWorkflow(def providers.Definition) bool {
	if !def.Capabilities.SupportsGuidedWorkflowDispatch {
		return false
	}
	if def.Capabilities.SupportsEvents {
		return true
	}
	return def.Capabilities.UsesItems
}
