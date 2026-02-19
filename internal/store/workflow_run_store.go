package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"control/internal/guidedworkflows"
)

const workflowRunSchemaVersion = 1

type WorkflowRunStore interface {
	ListWorkflowRuns(ctx context.Context) ([]guidedworkflows.RunStatusSnapshot, error)
	UpsertWorkflowRun(ctx context.Context, snapshot guidedworkflows.RunStatusSnapshot) error
}

type FileWorkflowRunStore struct {
	path string
	mu   sync.Mutex
}

type workflowRunFile struct {
	Version int                                 `json:"version"`
	Runs    []guidedworkflows.RunStatusSnapshot `json:"runs"`
}

func NewFileWorkflowRunStore(path string) *FileWorkflowRunStore {
	return &FileWorkflowRunStore{path: path}
}

func (s *FileWorkflowRunStore) ListWorkflowRuns(ctx context.Context) ([]guidedworkflows.RunStatusSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := s.load()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []guidedworkflows.RunStatusSnapshot{}, nil
		}
		return nil, err
	}
	out := make([]guidedworkflows.RunStatusSnapshot, 0, len(file.Runs))
	for _, snapshot := range file.Runs {
		out = append(out, cloneRunStatusSnapshot(snapshot))
	}
	sort.Slice(out, func(i, j int) bool {
		return runSnapshotSortTime(out[i]).After(runSnapshotSortTime(out[j]))
	})
	return out, nil
}

func (s *FileWorkflowRunStore) UpsertWorkflowRun(ctx context.Context, snapshot guidedworkflows.RunStatusSnapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	normalized, err := normalizeRunStatusSnapshot(snapshot)
	if err != nil {
		return err
	}
	file, err := s.load()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if file == nil {
		file = newWorkflowRunFile()
	}
	replaced := false
	for i := range file.Runs {
		existing := file.Runs[i]
		if existing.Run == nil || strings.TrimSpace(existing.Run.ID) != normalized.Run.ID {
			continue
		}
		file.Runs[i] = normalized
		replaced = true
		break
	}
	if !replaced {
		file.Runs = append(file.Runs, normalized)
	}
	return s.save(file)
}

func (s *FileWorkflowRunStore) load() (*workflowRunFile, error) {
	file := newWorkflowRunFile()
	if err := readJSON(s.path, file); err != nil {
		return nil, err
	}
	if file.Version == 0 {
		file.Version = workflowRunSchemaVersion
	}
	if file.Runs == nil {
		file.Runs = []guidedworkflows.RunStatusSnapshot{}
	}
	return file, nil
}

func (s *FileWorkflowRunStore) save(file *workflowRunFile) error {
	if file == nil {
		return errors.New("workflow run file is required")
	}
	file.Version = workflowRunSchemaVersion
	if file.Runs == nil {
		file.Runs = []guidedworkflows.RunStatusSnapshot{}
	}
	return writeJSONAtomic(s.path, file)
}

func newWorkflowRunFile() *workflowRunFile {
	return &workflowRunFile{
		Version: workflowRunSchemaVersion,
		Runs:    []guidedworkflows.RunStatusSnapshot{},
	}
}

func normalizeRunStatusSnapshot(snapshot guidedworkflows.RunStatusSnapshot) (guidedworkflows.RunStatusSnapshot, error) {
	if snapshot.Run == nil || strings.TrimSpace(snapshot.Run.ID) == "" {
		return guidedworkflows.RunStatusSnapshot{}, errors.New("workflow run snapshot requires run id")
	}
	out := cloneRunStatusSnapshot(snapshot)
	out.Run.ID = strings.TrimSpace(out.Run.ID)
	if out.Timeline == nil {
		out.Timeline = []guidedworkflows.RunTimelineEvent{}
	}
	for i := range out.Timeline {
		out.Timeline[i].RunID = strings.TrimSpace(out.Timeline[i].RunID)
		if out.Timeline[i].RunID == "" {
			out.Timeline[i].RunID = out.Run.ID
		}
	}
	return out, nil
}

func cloneRunStatusSnapshot(snapshot guidedworkflows.RunStatusSnapshot) guidedworkflows.RunStatusSnapshot {
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return guidedworkflows.RunStatusSnapshot{}
	}
	var out guidedworkflows.RunStatusSnapshot
	if err := json.Unmarshal(raw, &out); err != nil {
		return guidedworkflows.RunStatusSnapshot{}
	}
	return out
}

func runSnapshotSortTime(snapshot guidedworkflows.RunStatusSnapshot) time.Time {
	if snapshot.Run == nil {
		return time.Time{}
	}
	latest := snapshot.Run.CreatedAt
	if snapshot.Run.StartedAt != nil && snapshot.Run.StartedAt.After(latest) {
		latest = *snapshot.Run.StartedAt
	}
	if snapshot.Run.PausedAt != nil && snapshot.Run.PausedAt.After(latest) {
		latest = *snapshot.Run.PausedAt
	}
	if snapshot.Run.CompletedAt != nil && snapshot.Run.CompletedAt.After(latest) {
		latest = *snapshot.Run.CompletedAt
	}
	if snapshot.Run.DismissedAt != nil && snapshot.Run.DismissedAt.After(latest) {
		latest = *snapshot.Run.DismissedAt
	}
	return latest
}
