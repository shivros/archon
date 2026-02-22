package daemon

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCodexThreadTurnActivityProbe(t *testing.T) {
	probe := codexThreadTurnActivityProbe{timeout: 200 * time.Millisecond}
	tests := []struct {
		name     string
		reader   codexTurnReader
		threadID string
		turnID   string
		want     turnActivityStatus
		wantErr  bool
	}{
		{
			name: "active turn",
			reader: stubCodexTurnReader{
				thread: &codexThread{Turns: []codexTurn{{ID: "turn-1", Status: "in_progress"}}},
			},
			threadID: "thr-1",
			turnID:   "turn-1",
			want:     turnActivityActive,
		},
		{
			name: "inactive turn",
			reader: stubCodexTurnReader{
				thread: &codexThread{Turns: []codexTurn{{ID: "turn-1", Status: "completed"}}},
			},
			threadID: "thr-1",
			turnID:   "turn-1",
			want:     turnActivityInactive,
		},
		{
			name: "missing status is unknown",
			reader: stubCodexTurnReader{
				thread: &codexThread{Turns: []codexTurn{{ID: "turn-1"}}},
			},
			threadID: "thr-1",
			turnID:   "turn-1",
			want:     turnActivityUnknown,
		},
		{
			name: "missing turn is unknown",
			reader: stubCodexTurnReader{
				thread: &codexThread{Turns: []codexTurn{{ID: "turn-2", Status: "completed"}}},
			},
			threadID: "thr-1",
			turnID:   "turn-1",
			want:     turnActivityUnknown,
		},
		{
			name: "read error is unknown",
			reader: stubCodexTurnReader{
				err: errors.New("boom"),
			},
			threadID: "thr-1",
			turnID:   "turn-1",
			want:     turnActivityUnknown,
			wantErr:  true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := probe.Probe(context.Background(), tc.reader, tc.threadID, tc.turnID)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

type stubCodexTurnReader struct {
	thread *codexThread
	err    error
}

func (s stubCodexTurnReader) ReadThread(context.Context, string) (*codexThread, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.thread, nil
}
