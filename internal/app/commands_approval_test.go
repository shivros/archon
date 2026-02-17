package app

import (
	"context"
	"testing"

	"control/internal/client"
)

type stubApproveSessionAPI struct {
	id  string
	req client.ApproveSessionRequest
	err error
}

func (s *stubApproveSessionAPI) ApproveSession(_ context.Context, id string, req client.ApproveSessionRequest) error {
	s.id = id
	s.req = req
	return s.err
}

func TestApproveSessionCmdIncludesResponses(t *testing.T) {
	api := &stubApproveSessionAPI{}

	cmd := approveSessionCmd(api, "s1", 7, "accept", []string{"because tests"})
	if cmd == nil {
		t.Fatalf("expected command")
	}
	msg := cmd()
	result, ok := msg.(approvalMsg)
	if !ok {
		t.Fatalf("expected approvalMsg, got %T", msg)
	}
	if result.err != nil {
		t.Fatalf("unexpected approval error: %v", result.err)
	}
	if api.id != "s1" {
		t.Fatalf("expected session id s1, got %q", api.id)
	}
	if api.req.RequestID != 7 || api.req.Decision != "accept" {
		t.Fatalf("unexpected request payload: %#v", api.req)
	}
	if len(api.req.Responses) != 1 || api.req.Responses[0] != "because tests" {
		t.Fatalf("unexpected responses payload: %#v", api.req.Responses)
	}
}
