package guidedworkflows

import (
	"errors"
	"strings"
	"testing"

	"control/internal/providers"
)

func TestDispatchProviderPolicyUsesProviderCapabilities(t *testing.T) {
	policy := DefaultDispatchProviderPolicy()
	for _, def := range providers.All() {
		got := policy.SupportsDispatch(def.Name)
		if got != CanDispatchGuidedWorkflow(def) {
			t.Fatalf("provider %q dispatch support mismatch: got=%v want=%v", def.Name, got, CanDispatchGuidedWorkflow(def))
		}
	}
}

func TestCanDispatchGuidedWorkflowRequiresDispatchCapability(t *testing.T) {
	def := providers.Definition{
		Name: "x",
		Capabilities: providers.Capabilities{
			SupportsEvents: true,
		},
	}
	if CanDispatchGuidedWorkflow(def) {
		t.Fatalf("expected dispatch helper to require SupportsGuidedWorkflowDispatch")
	}
}

func TestCanDispatchGuidedWorkflowRequiresCompletionPath(t *testing.T) {
	def := providers.Definition{
		Name: "x",
		Capabilities: providers.Capabilities{
			SupportsGuidedWorkflowDispatch: true,
		},
	}
	if CanDispatchGuidedWorkflow(def) {
		t.Fatalf("expected dispatch helper to require events or items completion path")
	}
}

func TestCanDispatchGuidedWorkflowAllowsEventsOrItems(t *testing.T) {
	eventsDef := providers.Definition{
		Name: "events",
		Capabilities: providers.Capabilities{
			SupportsGuidedWorkflowDispatch: true,
			SupportsEvents:                 true,
		},
	}
	if !CanDispatchGuidedWorkflow(eventsDef) {
		t.Fatalf("expected events provider to be dispatchable")
	}
	itemsDef := providers.Definition{
		Name: "items",
		Capabilities: providers.Capabilities{
			SupportsGuidedWorkflowDispatch: true,
			UsesItems:                      true,
		},
	}
	if !CanDispatchGuidedWorkflow(itemsDef) {
		t.Fatalf("expected items provider to be dispatchable")
	}
}

func TestDispatchProviderValidationIsStructured(t *testing.T) {
	policy := DefaultDispatchProviderPolicy()
	if err := policy.Validate(""); err != nil {
		t.Fatalf("expected empty provider to pass validation, got %v", err)
	}
	if err := policy.Validate("codex"); err != nil {
		t.Fatalf("expected codex to pass validation, got %v", err)
	}
	if err := policy.Validate("claude"); err != nil {
		t.Fatalf("expected claude to pass validation, got %v", err)
	}
	err := policy.Validate("gemini")
	if err == nil {
		t.Fatalf("expected gemini to fail validation")
	}
	if !errors.Is(err, ErrUnsupportedProvider) {
		t.Fatalf("expected ErrUnsupportedProvider, got %v", err)
	}
	provider, ok := UnsupportedDispatchProvider(err)
	if !ok {
		t.Fatalf("expected structured provider details, got %T", err)
	}
	if provider != "gemini" {
		t.Fatalf("expected normalized provider gemini, got %q", provider)
	}
}

func TestDispatchProviderValidationErrorNilReceiver(t *testing.T) {
	var err *DispatchProviderValidationError
	if got := err.Error(); got == "" {
		t.Fatalf("expected fallback error message for nil receiver")
	}
}

func TestDispatchProviderValidationErrorNonNilReceiver(t *testing.T) {
	err := &DispatchProviderValidationError{Provider: "claude"}
	if got := err.Error(); got == "" || !strings.Contains(got, "claude") {
		t.Fatalf("expected provider in error message, got %q", got)
	}
}

func TestUnsupportedDispatchProviderHandlesTypedNil(t *testing.T) {
	var typedNil *DispatchProviderValidationError
	var err error = typedNil
	if provider, ok := UnsupportedDispatchProvider(err); ok || provider != "" {
		t.Fatalf("expected typed nil to return no provider, got provider=%q ok=%v", provider, ok)
	}
}

func TestUnsupportedDispatchProviderHandlesNilAndGenericError(t *testing.T) {
	if provider, ok := UnsupportedDispatchProvider(nil); ok || provider != "" {
		t.Fatalf("expected nil error to return no provider, got provider=%q ok=%v", provider, ok)
	}
	if provider, ok := UnsupportedDispatchProvider(errors.New("boom")); ok || provider != "" {
		t.Fatalf("expected generic error to return no provider, got provider=%q ok=%v", provider, ok)
	}
}

func TestDispatchProviderWrappersUseDefaultPolicy(t *testing.T) {
	if got := NormalizeDispatchProvider(" CoDeX "); got != "codex" {
		t.Fatalf("expected normalized codex, got %q", got)
	}
	if !SupportsDispatchProvider("codex") {
		t.Fatalf("expected codex to be dispatchable")
	}
	if !SupportsDispatchProvider("claude") {
		t.Fatalf("expected claude to be dispatchable")
	}
	if err := ValidateDispatchProvider("codex"); err != nil {
		t.Fatalf("expected codex validation success, got %v", err)
	}
}
