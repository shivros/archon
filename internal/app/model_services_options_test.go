package app

import "testing"

type stubApprovalStateService struct {
	latest *ApprovalRequest
}

func (s stubApprovalStateService) SetRequests(_ string, state ApprovalSessionState, _ []*ApprovalRequest) ApprovalSessionState {
	return state
}
func (s stubApprovalStateService) UpsertRequest(_ string, state ApprovalSessionState, _ *ApprovalRequest) (ApprovalSessionState, bool) {
	return state, false
}
func (s stubApprovalStateService) RemoveRequest(state ApprovalSessionState, _ int) (ApprovalSessionState, bool) {
	return state, false
}
func (s stubApprovalStateService) UpsertResolution(_ string, state ApprovalSessionState, _ *ApprovalResolution) (ApprovalSessionState, bool) {
	return state, false
}
func (s stubApprovalStateService) RemoveResolution(state ApprovalSessionState, _ int) (ApprovalSessionState, bool) {
	return state, false
}
func (s stubApprovalStateService) LatestRequest(_ []*ApprovalRequest) *ApprovalRequest {
	return s.latest
}

type stubTranscriptComposer struct{}

func (stubTranscriptComposer) AppendOptimisticUser(base []ChatBlock, _ string) ([]ChatBlock, int) {
	return base, -1
}
func (stubTranscriptComposer) MarkUserStatus(base []ChatBlock, _ int, _ ChatStatus) ([]ChatBlock, bool) {
	return base, false
}
func (stubTranscriptComposer) MergeApprovals(base []ChatBlock, _ []*ApprovalRequest, _ []*ApprovalResolution, _ []ChatBlock) []ChatBlock {
	return base
}

func TestWithApprovalStateServiceConfiguresAndResetsDefault(t *testing.T) {
	custom := stubApprovalStateService{latest: &ApprovalRequest{RequestID: 8}}
	m := NewModel(nil, WithApprovalStateService(custom))
	if _, ok := m.approvalStateService.(stubApprovalStateService); !ok {
		t.Fatalf("expected custom approval service, got %T", m.approvalStateService)
	}
	if req := m.approvalStateServiceOrDefault().LatestRequest(nil); req == nil || req.RequestID != 8 {
		t.Fatalf("expected custom approval service in fallback resolver")
	}

	WithApprovalStateService(nil)(&m)
	if m.approvalStateService != nil {
		t.Fatalf("expected nil option to clear approval service")
	}
	if m.approvalStateServiceOrDefault() == nil {
		t.Fatalf("expected default approval service fallback")
	}
}

func TestWithApprovalStateServiceHandlesNilModel(t *testing.T) {
	WithApprovalStateService(stubApprovalStateService{})(nil)
	var nilModel *Model
	if nilModel.approvalStateServiceOrDefault() == nil {
		t.Fatalf("expected nil model to resolve default approval service")
	}
}

func TestWithTranscriptComposerConfiguresAndResetsDefault(t *testing.T) {
	custom := stubTranscriptComposer{}
	m := NewModel(nil, WithTranscriptComposer(custom))
	if _, ok := m.transcriptComposer.(stubTranscriptComposer); !ok {
		t.Fatalf("expected custom transcript composer, got %T", m.transcriptComposer)
	}
	if _, ok := m.transcriptComposerOrDefault().(stubTranscriptComposer); !ok {
		t.Fatalf("expected custom transcript composer in fallback resolver")
	}

	WithTranscriptComposer(nil)(&m)
	if m.transcriptComposer != nil {
		t.Fatalf("expected nil option to clear transcript composer")
	}
	if m.transcriptComposerOrDefault() == nil {
		t.Fatalf("expected default transcript composer fallback")
	}
}

func TestWithTranscriptComposerHandlesNilModel(t *testing.T) {
	WithTranscriptComposer(stubTranscriptComposer{})(nil)
	var nilModel *Model
	if nilModel.transcriptComposerOrDefault() == nil {
		t.Fatalf("expected nil model to resolve default transcript composer")
	}
}

func TestModelApprovalWrappersCoverStateServicePaths(t *testing.T) {
	m := NewModel(nil)
	m.sessionApprovals["s1"] = []*ApprovalRequest{}
	m.sessionApprovalResolutions["s1"] = []*ApprovalResolution{
		{RequestID: 1, SessionID: "s1"},
	}

	if changed := m.upsertApprovalForSession("s1", &ApprovalRequest{RequestID: 1, Summary: "command"}); !changed {
		t.Fatalf("expected upsert approval wrapper to report change")
	}
	if len(m.sessionApprovalResolutions["s1"]) != 0 {
		t.Fatalf("expected upsert wrapper to clear matching resolution")
	}

	m.sessionApprovalResolutions["s1"] = []*ApprovalResolution{
		{RequestID: 4, SessionID: "s1"},
	}
	if changed := m.removeApprovalResolutionForSession("s1", 4); !changed {
		t.Fatalf("expected remove resolution wrapper to report change")
	}
	if len(m.sessionApprovalResolutions["s1"]) != 0 {
		t.Fatalf("expected resolution removal wrapper to clear entry")
	}
}
