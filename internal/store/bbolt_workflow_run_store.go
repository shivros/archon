package store

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"

	bolt "go.etcd.io/bbolt"

	"control/internal/guidedworkflows"
)

type bboltWorkflowRunStore struct {
	db *bolt.DB
	mu sync.Mutex
}

func (s *bboltWorkflowRunStore) ListWorkflowRuns(ctx context.Context) ([]guidedworkflows.RunStatusSnapshot, error) {
	out := make([]guidedworkflows.RunStatusSnapshot, 0)
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketWorkflowRuns)
		if b == nil {
			return nil
		}
		return b.ForEach(func(_, v []byte) error {
			var snapshot guidedworkflows.RunStatusSnapshot
			if err := json.Unmarshal(v, &snapshot); err != nil {
				return err
			}
			out = append(out, cloneRunStatusSnapshot(snapshot))
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		return runSnapshotSortTime(out[i]).After(runSnapshotSortTime(out[j]))
	})
	return out, nil
}

func (s *bboltWorkflowRunStore) UpsertWorkflowRun(ctx context.Context, snapshot guidedworkflows.RunStatusSnapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	normalized, err := normalizeRunStatusSnapshot(snapshot)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(normalized)
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketWorkflowRuns)
		if b == nil {
			return errors.New("workflow_runs bucket missing")
		}
		runID := strings.TrimSpace(normalized.Run.ID)
		return b.Put([]byte(runID), raw)
	})
}
