package daemon

import (
	"context"
	"errors"
	"testing"

	"control/internal/types"
)

func TestValidateTurnLifecycleWiringRequiresOpenCodeFactories(t *testing.T) {
	manager := &CompositeLiveManager{
		factories: map[string]TurnCapableSessionFactory{},
	}
	if err := manager.ValidateLifecycleWiring("opencode", "kilocode"); err == nil {
		t.Fatalf("expected missing factory error")
	}
}

func TestValidateTurnLifecycleWiringAcceptsConfiguredFactories(t *testing.T) {
	manager := &CompositeLiveManager{
		factories: map[string]TurnCapableSessionFactory{
			"opencode": &openCodeLiveSessionFactory{
				turnNotifier: NopTurnCompletionNotifier{},
				repository:   &stubTurnArtifactRepository{},
				payloads:     defaultTurnCompletionPayloadBuilder{},
				freshness:    NewTurnEvidenceFreshnessTracker(),
			},
			"kilocode": &openCodeLiveSessionFactory{
				turnNotifier: NopTurnCompletionNotifier{},
				repository:   &stubTurnArtifactRepository{},
				payloads:     defaultTurnCompletionPayloadBuilder{},
				freshness:    NewTurnEvidenceFreshnessTracker(),
			},
		},
	}
	if err := manager.ValidateLifecycleWiring("opencode", "kilocode"); err != nil {
		t.Fatalf("expected lifecycle wiring validation to pass, got %v", err)
	}
}

func TestValidateTurnLifecycleWiringUsesFactoryValidatorInterface(t *testing.T) {
	manager := &CompositeLiveManager{
		factories: map[string]TurnCapableSessionFactory{
			"opencode": fakeValidatorFactory{provider: "opencode", err: errors.New("bad wiring")},
		},
	}
	if err := manager.ValidateLifecycleWiring("opencode"); err == nil {
		t.Fatalf("expected validator error")
	}
}

func TestValidateTurnLifecycleWiringNormalizesProviderNames(t *testing.T) {
	manager := &CompositeLiveManager{
		factories: map[string]TurnCapableSessionFactory{
			"opencode": &openCodeLiveSessionFactory{
				turnNotifier: NopTurnCompletionNotifier{},
				repository:   &stubTurnArtifactRepository{},
				payloads:     defaultTurnCompletionPayloadBuilder{},
				freshness:    NewTurnEvidenceFreshnessTracker(),
			},
		},
	}
	if err := manager.ValidateLifecycleWiring("  OpenCode  "); err != nil {
		t.Fatalf("expected normalized provider name to validate, got %v", err)
	}
}

func TestValidateTurnLifecycleWiringAllowsFactoriesWithoutValidator(t *testing.T) {
	manager := &CompositeLiveManager{
		factories: map[string]TurnCapableSessionFactory{
			"opencode": nonValidatingFactory{provider: "opencode"},
		},
	}
	if err := manager.ValidateLifecycleWiring("opencode"); err != nil {
		t.Fatalf("expected non-validating factory to pass, got %v", err)
	}
}

func TestValidateTurnLifecycleWiringFailsWhenOneRequiredProviderMissing(t *testing.T) {
	manager := &CompositeLiveManager{
		factories: map[string]TurnCapableSessionFactory{
			"opencode": nonValidatingFactory{provider: "opencode"},
		},
	}
	if err := manager.ValidateLifecycleWiring("opencode", "kilocode"); err == nil {
		t.Fatalf("expected missing required provider error")
	}
}

type fakeValidatorFactory struct {
	provider string
	err      error
}

func (f fakeValidatorFactory) ProviderName() string { return f.provider }

func (f fakeValidatorFactory) CreateTurnCapable(context.Context, *types.Session, *types.SessionMeta) (TurnCapableSession, error) {
	return nil, errors.New("not implemented")
}

func (f fakeValidatorFactory) ValidateLifecycleWiring() error { return f.err }

type nonValidatingFactory struct {
	provider string
}

func (f nonValidatingFactory) ProviderName() string { return f.provider }

func (f nonValidatingFactory) CreateTurnCapable(context.Context, *types.Session, *types.SessionMeta) (TurnCapableSession, error) {
	return nil, errors.New("not implemented")
}
