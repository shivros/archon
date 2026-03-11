package app

import "testing"

func TestWithTranscriptRenderProjectorOption(t *testing.T) {
	custom := NewDefaultTranscriptRenderProjector()
	model := NewModel(nil, WithTranscriptRenderProjector(custom))
	if model.transcriptRenderProjector != custom {
		t.Fatalf("expected custom render projector to be installed")
	}

	model = NewModel(nil, WithTranscriptRenderProjector(nil))
	if model.transcriptRenderProjector == nil {
		t.Fatalf("expected nil option to install default render projector")
	}
	if model.transcriptRenderProjectorOrDefault() == nil {
		t.Fatalf("expected render projector fallback to be non-nil")
	}
}
