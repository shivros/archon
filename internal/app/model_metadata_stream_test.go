package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"control/internal/guidedworkflows"
	"control/internal/types"
)

type metadataRecoveryPolicyStub struct {
	onErrorDecision     MetadataStreamRecoveryDecision
	onClosedDecision    MetadataStreamRecoveryDecision
	onConnectedDecision MetadataStreamRecoveryDecision
}

func (s metadataRecoveryPolicyStub) OnError(int) MetadataStreamRecoveryDecision {
	return s.onErrorDecision
}

func (s metadataRecoveryPolicyStub) OnClosed(int) MetadataStreamRecoveryDecision {
	return s.onClosedDecision
}

func (s metadataRecoveryPolicyStub) OnConnected() MetadataStreamRecoveryDecision {
	return s.onConnectedDecision
}

type metadataStreamAPIStub struct {
	afterSeen string
	ch        <-chan types.MetadataEvent
	cancel    func()
	err       error
}

func (s *metadataStreamAPIStub) MetadataStream(_ context.Context, afterRevision string) (<-chan types.MetadataEvent, func(), error) {
	s.afterSeen = afterRevision
	if s.err != nil {
		return nil, nil, s.err
	}
	cancel := s.cancel
	if cancel == nil {
		cancel = func() {}
	}
	return s.ch, cancel, nil
}

func TestApplyMetadataEventPatchesSessionAndWorkflowTitles(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	now := time.Now().UTC()
	m.workflowRuns = []*guidedworkflows.WorkflowRun{
		{ID: "gwf-1", TemplateName: "Before Workflow", CreatedAt: now},
	}
	m.sessionMeta["s1"].Title = "Before Session"

	m.applyMetadataEvent(types.MetadataEvent{
		Type:     types.MetadataEventTypeSessionUpdated,
		Revision: "10",
		Session: &types.MetadataEntityUpdated{
			ID:        "s1",
			Title:     "After Session",
			UpdatedAt: now,
		},
	})
	m.applyMetadataEvent(types.MetadataEvent{
		Type:     types.MetadataEventTypeWorkflowRunUpdated,
		Revision: "11",
		Workflow: &types.MetadataEntityUpdated{
			ID:        "gwf-1",
			Title:     "After Workflow",
			UpdatedAt: now,
		},
	})

	if got := m.sessionMeta["s1"].Title; got != "After Session" {
		t.Fatalf("expected session title patch, got %q", got)
	}
	if got := m.workflowRuns[0].TemplateName; got != "After Workflow" {
		t.Fatalf("expected workflow title patch, got %q", got)
	}
	if got := m.metadataStreamRevision; got != "11" {
		t.Fatalf("expected revision update, got %q", got)
	}
}

func TestApplyMetadataStreamMsgErrorSchedulesReconnect(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	cmd := m.applyMetadataStreamMsg(metadataStreamMsg{err: errors.New("boom")})
	if cmd == nil {
		t.Fatalf("expected reconnect command on metadata stream error")
	}
	if m.metadataStreamReconnectAttempts == 0 {
		t.Fatalf("expected reconnect attempts to increment")
	}
}

func TestReduceStateMessagesMetadataReconnectUsesLastRevision(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	ch := make(chan types.MetadataEvent)
	close(ch)
	stub := &metadataStreamAPIStub{ch: ch}
	m.metadataStreamAPI = stub
	m.metadataStreamRevision = "42"

	handled, cmd := m.reduceStateMessages(metadataStreamReconnectMsg{})
	if !handled {
		t.Fatalf("expected reconnect message to be handled")
	}
	if cmd == nil {
		t.Fatalf("expected reconnect command")
	}
	msg, ok := cmd().(metadataStreamMsg)
	if !ok {
		t.Fatalf("expected metadataStreamMsg, got %T", cmd())
	}
	if msg.err != nil {
		t.Fatalf("unexpected stream error: %v", msg.err)
	}
	if stub.afterSeen != "42" {
		t.Fatalf("expected reconnect after revision 42, got %q", stub.afterSeen)
	}
}

func TestConsumeMetadataTickClosedReturnsReconnectCmd(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	ch := make(chan types.MetadataEvent)
	close(ch)
	m.metadataStream.SetStream(ch, nil)
	cmd := m.consumeMetadataTick(time.Now().UTC())
	if cmd == nil {
		t.Fatalf("expected reconnect command on stream close")
	}
}

func TestConsumeMetadataTickClosedUsesRecoveryPolicyAttempts(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.metadataStreamRecoveryPolicy = metadataRecoveryPolicyStub{
		onClosedDecision: MetadataStreamRecoveryDecision{
			NextAttempts:   9,
			ReconnectDelay: 5 * time.Millisecond,
		},
	}
	ch := make(chan types.MetadataEvent)
	close(ch)
	m.metadataStream.SetStream(ch, nil)
	cmd := m.consumeMetadataTick(time.Now().UTC())
	if cmd == nil {
		t.Fatalf("expected reconnect command on stream close")
	}
	if m.metadataStreamReconnectAttempts != 9 {
		t.Fatalf("expected attempts from policy, got %d", m.metadataStreamReconnectAttempts)
	}
}

func TestApplyMetadataStreamMsgErrorUsesRecoveryPolicyAttempts(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.metadataStreamRecoveryPolicy = metadataRecoveryPolicyStub{
		onErrorDecision: MetadataStreamRecoveryDecision{
			NextAttempts:   7,
			ReconnectDelay: 5 * time.Millisecond,
			RefreshLists:   true,
		},
	}
	cmd := m.applyMetadataStreamMsg(metadataStreamMsg{err: errors.New("boom")})
	if cmd == nil {
		t.Fatalf("expected reconnect command on metadata stream error")
	}
	if m.metadataStreamReconnectAttempts != 7 {
		t.Fatalf("expected attempts from policy, got %d", m.metadataStreamReconnectAttempts)
	}
}
