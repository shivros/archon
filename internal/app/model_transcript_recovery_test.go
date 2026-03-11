package app

import (
	"context"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"control/internal/client"
	"control/internal/daemon/transcriptdomain"
)

type captureTranscriptRecoveryScheduler struct {
	calls    int
	sessions []string
	plan     TranscriptRecoveryPlan
}

func (s *captureTranscriptRecoveryScheduler) Plan(request TranscriptRecoveryRequest) TranscriptRecoveryPlan {
	s.calls++
	s.sessions = append(s.sessions, request.SessionID)
	if s.plan != (TranscriptRecoveryPlan{}) {
		return s.plan
	}
	return TranscriptRecoveryPlan{
		FetchTranscriptSnapshot: true,
		FetchApprovals:          true,
		SnapshotSource:          transcriptAttachmentSourceRecovery,
		AuthoritativeSnapshot:   true,
	}
}

type transcriptRecoveryHistoryStub struct {
	calls int
}

func (s *transcriptRecoveryHistoryStub) History(context.Context, string, int) (*client.TailItemsResponse, error) {
	s.calls++
	return &client.TailItemsResponse{Items: nil}, nil
}

func executeTeaCmd(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	return flattenTeaMsg(cmd())
}

func flattenTeaMsg(msg tea.Msg) []tea.Msg {
	switch typed := msg.(type) {
	case nil:
		return nil
	case tea.BatchMsg:
		out := make([]tea.Msg, 0, len(typed))
		for _, cmd := range typed {
			out = append(out, executeTeaCmd(cmd)...)
		}
		return out
	default:
		return []tea.Msg{typed}
	}
}

func TestMaybeRecoverTranscriptFromControlOnlySignalsTriggersAfterSustainedBatches(t *testing.T) {
	m := NewModel(nil)
	m.enterCompose("s1")
	m.startRequestActivity("s1", "codex")
	m.requestActivity.lastVisibleAt = time.Now().UTC().Add(-10 * time.Second)
	scheduler := &captureTranscriptRecoveryScheduler{}
	m.transcriptRecoveryScheduler = scheduler

	now := time.Now().UTC()
	if cmd := m.maybeRecoverTranscriptFromControlOnlySignals(now, "s1", "codex", TranscriptTickSignals{
		Events:        6,
		ControlEvents: 6,
	}); cmd != nil {
		t.Fatalf("expected first control-only batch not to trigger recovery")
	}
	cmd := m.maybeRecoverTranscriptFromControlOnlySignals(now.Add(1*time.Second), "s1", "codex", TranscriptTickSignals{
		Events:        6,
		ControlEvents: 6,
	})
	if cmd == nil {
		t.Fatalf("expected second sustained control-only batch to trigger recovery")
	}
	if scheduler.calls != 1 {
		t.Fatalf("expected one recovery call, got %d", scheduler.calls)
	}
	if scheduler.sessions[0] != "s1" {
		t.Fatalf("expected recovery for s1, got %#v", scheduler.sessions)
	}
}

func TestMaybeRecoverTranscriptFromControlOnlySignalsRespectsCooldownAndContentReset(t *testing.T) {
	m := NewModel(nil)
	m.enterCompose("s1")
	m.startRequestActivity("s1", "codex")
	m.requestActivity.lastVisibleAt = time.Now().UTC().Add(-10 * time.Second)
	scheduler := &captureTranscriptRecoveryScheduler{}
	m.transcriptRecoveryScheduler = scheduler

	base := time.Now().UTC()
	_ = m.maybeRecoverTranscriptFromControlOnlySignals(base, "s1", "codex", TranscriptTickSignals{Events: 6, ControlEvents: 6})
	_ = m.maybeRecoverTranscriptFromControlOnlySignals(base.Add(1*time.Second), "s1", "codex", TranscriptTickSignals{Events: 6, ControlEvents: 6})
	if scheduler.calls != 1 {
		t.Fatalf("expected initial recovery call, got %d", scheduler.calls)
	}
	if cmd := m.maybeRecoverTranscriptFromControlOnlySignals(base.Add(2*time.Second), "s1", "codex", TranscriptTickSignals{Events: 6, ControlEvents: 6}); cmd != nil {
		t.Fatalf("expected cooldown to suppress immediate re-recovery")
	}
	if scheduler.calls != 1 {
		t.Fatalf("expected no additional recovery call during cooldown, got %d", scheduler.calls)
	}

	_ = m.maybeRecoverTranscriptFromControlOnlySignals(base.Add(5*time.Second), "s1", "codex", TranscriptTickSignals{Events: 1, ContentEvents: 1})
	if cmd := m.maybeRecoverTranscriptFromControlOnlySignals(base.Add(6*time.Second), "s1", "codex", TranscriptTickSignals{Events: 6, ControlEvents: 6}); cmd != nil {
		t.Fatalf("expected post-content first control batch not to recover")
	}
	cmd := m.maybeRecoverTranscriptFromControlOnlySignals(base.Add(7*time.Second), "s1", "codex", TranscriptTickSignals{Events: 6, ControlEvents: 6})
	if cmd == nil {
		t.Fatalf("expected second post-content control batch to recover")
	}
	if scheduler.calls != 2 {
		t.Fatalf("expected second recovery call after reset, got %d", scheduler.calls)
	}
}

func TestMaybeRecoverTranscriptFromRevisionRewindTriggersImmediateRecovery(t *testing.T) {
	m := NewModel(nil)
	m.enterCompose("s1")
	m.startRequestActivity("s1", "codex")
	scheduler := &captureTranscriptRecoveryScheduler{}
	m.transcriptRecoveryScheduler = scheduler

	cmd := m.maybeRecoverTranscriptFromRevisionRewind(time.Now().UTC(), "s1", "codex", TranscriptTickSignals{
		Events:         1,
		ControlEvents:  1,
		RevisionRewind: true,
	})
	if cmd == nil {
		t.Fatalf("expected rewind to trigger recovery")
	}
	if scheduler.calls != 1 || scheduler.sessions[0] != "s1" {
		t.Fatalf("expected one rewind recovery for s1, got calls=%d sessions=%#v", scheduler.calls, scheduler.sessions)
	}
}

func TestMaybeRecoverTranscriptFromRevisionRewindRespectsCooldown(t *testing.T) {
	m := NewModel(nil)
	m.enterCompose("s1")
	m.startRequestActivity("s1", "codex")
	scheduler := &captureTranscriptRecoveryScheduler{}
	m.transcriptRecoveryScheduler = scheduler

	base := time.Now().UTC()
	_ = m.maybeRecoverTranscriptFromRevisionRewind(base, "s1", "codex", TranscriptTickSignals{RevisionRewind: true})
	if cmd := m.maybeRecoverTranscriptFromRevisionRewind(base.Add(time.Second), "s1", "codex", TranscriptTickSignals{RevisionRewind: true}); cmd != nil {
		t.Fatalf("expected rewind recovery cooldown to suppress immediate retry")
	}
	if scheduler.calls != 1 {
		t.Fatalf("expected a single recovery during cooldown, got %d", scheduler.calls)
	}
}

func TestMaybeRecoverTranscriptFromRevisionRewindMarksGenerationUnhealthyAndDetaches(t *testing.T) {
	m := NewModel(nil)
	m.enterCompose("s1")
	m.startRequestActivity("s1", "codex")
	scheduler := &captureTranscriptRecoveryScheduler{}
	m.transcriptRecoveryScheduler = scheduler

	coordinator := m.transcriptAttachmentCoordinatorOrDefault()
	attachment := coordinator.Begin("s1", transcriptAttachmentSourceRecovery, "8")
	if m.transcriptStream != nil {
		m.transcriptStream.SetStreamWithGeneration(make(chan transcriptdomain.TranscriptEvent), func() {}, attachment.Generation)
	}

	cmd := m.maybeRecoverTranscriptFromRevisionRewind(time.Now().UTC(), "s1", "codex", TranscriptTickSignals{
		RevisionRewind: true,
		Generation:     attachment.Generation,
	})
	if cmd == nil {
		t.Fatalf("expected rewind recovery command")
	}
	if m.transcriptStream != nil && m.transcriptStream.HasStream() {
		t.Fatalf("expected rewound generation stream to detach")
	}
	decision := coordinator.Evaluate("s1", attachment.Generation)
	if decision.Accept {
		t.Fatalf("expected rewound generation to be marked unhealthy")
	}
	if !m.transcriptRecoveryCoordinatorOrDefault().ShouldApplyAuthoritativeSnapshot("s1") {
		t.Fatalf("expected rewind to require authoritative snapshot")
	}
}

func TestScheduleTranscriptRecoveryFallsBackToHistoryWhenSnapshotAPIUnavailable(t *testing.T) {
	m := NewModel(nil)
	m.pendingSessionKey = "sess:s1"
	history := &transcriptRecoveryHistoryStub{}
	m.sessionHistoryAPI = history
	m.transcriptRecoveryScheduler = &captureTranscriptRecoveryScheduler{
		plan: TranscriptRecoveryPlan{
			FetchTranscriptSnapshot: true,
			FetchHistory:            true,
		},
	}

	msgs := executeTeaCmd(m.scheduleTranscriptRecovery("s1", "codex"))
	if len(msgs) != 1 {
		t.Fatalf("expected one fallback recovery command, got %#v", msgs)
	}
	if _, ok := msgs[0].(historyMsg); !ok {
		t.Fatalf("expected history fallback command, got %T", msgs[0])
	}
	if history.calls != 1 {
		t.Fatalf("expected one history call, got %d", history.calls)
	}
}

func TestScheduleTranscriptRecoveryIncludesApprovalsWhenPolicyAllows(t *testing.T) {
	m := NewModel(nil)
	m.pendingSessionKey = "sess:s1"
	m.transcriptRecoveryScheduler = &captureTranscriptRecoveryScheduler{
		plan: TranscriptRecoveryPlan{
			FetchApprovals: true,
		},
	}

	if cmd := m.scheduleTranscriptRecovery("s1", "codex"); cmd == nil {
		t.Fatalf("expected approvals-capable provider to schedule recovery command")
	}
	if cmd := m.scheduleTranscriptRecovery("s1", "provider-without-approvals"); cmd != nil {
		t.Fatalf("expected approvals-unsupported provider not to schedule approvals command")
	}
}

func TestScheduleTranscriptRecoverySnapshotRequestPreservesSourceAndAuthority(t *testing.T) {
	m := NewModel(nil)
	m.pendingSessionKey = "sess:s1"
	m.sessionTranscriptAPI = bootstrapTranscriptAPIStub{}
	m.transcriptRecoveryScheduler = &captureTranscriptRecoveryScheduler{
		plan: TranscriptRecoveryPlan{
			FetchTranscriptSnapshot: true,
			SnapshotSource:          transcriptAttachmentSourceRecovery,
			AuthoritativeSnapshot:   true,
		},
	}

	msgs := executeTeaCmd(m.scheduleTranscriptRecovery("s1", "codex"))
	if len(msgs) != 1 {
		t.Fatalf("expected one snapshot recovery command, got %#v", msgs)
	}
	snapshotMsg, ok := msgs[0].(transcriptSnapshotMsg)
	if !ok {
		t.Fatalf("expected transcript snapshot command, got %T", msgs[0])
	}
	if snapshotMsg.source != transcriptAttachmentSourceRecovery || !snapshotMsg.authoritative {
		t.Fatalf("expected authoritative recovery snapshot request, got %#v", snapshotMsg)
	}
}
