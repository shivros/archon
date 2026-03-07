package app

import "testing"

func TestDefaultApprovalStateServiceSetRequestsNormalizesSessionID(t *testing.T) {
	service := NewDefaultApprovalStateService()
	state := ApprovalSessionState{
		Resolutions: []*ApprovalResolution{
			{RequestID: 5, SessionID: "s1"},
		},
	}

	next := service.SetRequests("s1", state, []*ApprovalRequest{
		{RequestID: 5, Summary: "command"},
	})

	if len(next.Requests) != 1 || next.Requests[0].SessionID != "s1" {
		t.Fatalf("expected normalized session id on request, got %#v", next.Requests)
	}
	if len(next.Resolutions) != 0 {
		t.Fatalf("expected stale resolution removal, got %#v", next.Resolutions)
	}
}

func TestDefaultApprovalStateServiceUpsertRequestClearsMatchingResolution(t *testing.T) {
	service := NewDefaultApprovalStateService()
	state := ApprovalSessionState{
		Requests: []*ApprovalRequest{
			{RequestID: 1, SessionID: "s1"},
		},
		Resolutions: []*ApprovalResolution{
			{RequestID: 7, SessionID: "s1"},
		},
	}

	next, changed := service.UpsertRequest("s1", state, &ApprovalRequest{RequestID: 7, Summary: "cmd"})
	if !changed {
		t.Fatalf("expected request upsert to be marked changed")
	}
	if len(next.Requests) != 2 {
		t.Fatalf("expected upserted request, got %#v", next.Requests)
	}
	if len(next.Resolutions) != 0 {
		t.Fatalf("expected matching resolution to clear, got %#v", next.Resolutions)
	}
}

func TestDefaultApprovalStateServiceUpsertResolutionNormalizesSessionID(t *testing.T) {
	service := NewDefaultApprovalStateService()
	state := ApprovalSessionState{}

	next, changed := service.UpsertResolution("s1", state, &ApprovalResolution{
		RequestID: 3,
		Decision:  "accept",
	})
	if !changed {
		t.Fatalf("expected resolution upsert to be marked changed")
	}
	if len(next.Resolutions) != 1 {
		t.Fatalf("expected one resolution, got %#v", next.Resolutions)
	}
	if next.Resolutions[0].SessionID != "s1" {
		t.Fatalf("expected normalized session id s1, got %#v", next.Resolutions[0])
	}
}

func TestDefaultApprovalStateServiceRemoveResolution(t *testing.T) {
	service := NewDefaultApprovalStateService()
	state := ApprovalSessionState{
		Resolutions: []*ApprovalResolution{
			{RequestID: 9, SessionID: "s1"},
		},
	}

	next, changed := service.RemoveResolution(state, 9)
	if !changed {
		t.Fatalf("expected remove to change state")
	}
	if len(next.Resolutions) != 0 {
		t.Fatalf("expected empty resolutions after remove, got %#v", next.Resolutions)
	}

	unchanged, changed := service.RemoveResolution(state, 999)
	if changed {
		t.Fatalf("expected missing request removal to be unchanged")
	}
	if len(unchanged.Resolutions) != 1 {
		t.Fatalf("expected original resolutions to remain, got %#v", unchanged.Resolutions)
	}
}
