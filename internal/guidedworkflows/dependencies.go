package guidedworkflows

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type dependencyValidator interface {
	NormalizeAndValidate(runID string, dependsOnRunIDs []string, existing map[string]*WorkflowRun) ([]RunDependency, error)
}

type dependencyGraphIndex interface {
	SetRun(run *WorkflowRun)
	RemoveRun(runID string)
	Dependents(runID string) []string
	Reset()
}

type dependencyEvaluator interface {
	Evaluate(now time.Time, run *WorkflowRun, lookup func(runID string) (*WorkflowRun, bool)) RunDependencyState
}

type queuedRunActivator interface {
	ShouldActivate(run *WorkflowRun, state RunDependencyState) bool
}

// Dependency collaborator interfaces stay package-internal.
// Interface introduction checklist:
// - each interface has an active production call site,
// - each interface has at least one test double,
// - each interface isolates a cohesive policy/algorithm boundary.

func NewDefaultDependencyValidator() dependencyValidator {
	return defaultDependencyValidator{}
}

func NewReverseDependencyGraphIndex() dependencyGraphIndex {
	return newReverseDependencyGraphIndex()
}

func NewDefaultDependencyEvaluator() dependencyEvaluator {
	return defaultDependencyEvaluator{}
}

func NewDefaultQueuedRunActivator() queuedRunActivator {
	return defaultQueuedRunActivator{}
}

type defaultDependencyValidator struct{}

func (defaultDependencyValidator) NormalizeAndValidate(
	runID string,
	dependsOnRunIDs []string,
	existing map[string]*WorkflowRun,
) ([]RunDependency, error) {
	runID = strings.TrimSpace(runID)
	if len(dependsOnRunIDs) == 0 {
		return nil, nil
	}
	normalized := make([]RunDependency, 0, len(dependsOnRunIDs))
	seen := map[string]struct{}{}
	for _, raw := range dependsOnRunIDs {
		depID := strings.TrimSpace(raw)
		if depID == "" {
			continue
		}
		if depID == runID {
			return nil, fmt.Errorf("%w: run %q cannot depend on itself", ErrDependencyGraph, runID)
		}
		if _, ok := seen[depID]; ok {
			continue
		}
		seen[depID] = struct{}{}
		if _, ok := existing[depID]; !ok {
			return nil, fmt.Errorf("%w: %s", ErrDependencyNotFound, depID)
		}
		normalized = append(normalized, RunDependency{
			RunID:     depID,
			Condition: DependencyConditionOnCompleted,
		})
	}
	if len(normalized) == 0 {
		return nil, nil
	}
	if hasDependencyPathToRun(runID, normalized, existing) {
		return nil, fmt.Errorf("%w: dependency cycle detected for run %q", ErrDependencyGraph, runID)
	}
	return normalized, nil
}

func hasDependencyPathToRun(
	runID string,
	runDependencies []RunDependency,
	existing map[string]*WorkflowRun,
) bool {
	target := strings.TrimSpace(runID)
	if target == "" {
		return false
	}
	adjacency := make(map[string][]string, len(existing)+1)
	for id, run := range existing {
		id = strings.TrimSpace(id)
		if id == "" || run == nil {
			continue
		}
		adjacency[id] = dependencyRunIDs(run.Dependencies)
	}
	adjacency[target] = dependencyRunIDs(runDependencies)
	for _, dep := range runDependencies {
		depID := strings.TrimSpace(dep.RunID)
		if depID == "" {
			continue
		}
		if hasPath(depID, target, adjacency, map[string]struct{}{}) {
			return true
		}
	}
	return false
}

func hasPath(current, target string, adjacency map[string][]string, seen map[string]struct{}) bool {
	current = strings.TrimSpace(current)
	target = strings.TrimSpace(target)
	if current == "" || target == "" {
		return false
	}
	if current == target {
		return true
	}
	if _, ok := seen[current]; ok {
		return false
	}
	seen[current] = struct{}{}
	for _, next := range adjacency[current] {
		next = strings.TrimSpace(next)
		if next == "" {
			continue
		}
		if hasPath(next, target, adjacency, seen) {
			return true
		}
	}
	return false
}

func dependencyRunIDs(dependencies []RunDependency) []string {
	if len(dependencies) == 0 {
		return nil
	}
	ids := make([]string, 0, len(dependencies))
	seen := map[string]struct{}{}
	for _, dep := range dependencies {
		id := strings.TrimSpace(dep.RunID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

func normalizeRunDependencies(raw []RunDependency) []RunDependency {
	if len(raw) == 0 {
		return nil
	}
	out := make([]RunDependency, 0, len(raw))
	seen := map[string]struct{}{}
	for _, dep := range raw {
		depID := strings.TrimSpace(dep.RunID)
		if depID == "" {
			continue
		}
		if _, ok := seen[depID]; ok {
			continue
		}
		seen[depID] = struct{}{}
		condition, ok := NormalizeDependencyCondition(string(dep.Condition))
		if !ok {
			condition = DependencyCondition(strings.ToLower(strings.TrimSpace(string(dep.Condition))))
		}
		out = append(out, RunDependency{
			RunID:     depID,
			Condition: condition,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

type reverseDependencyGraphIndex struct {
	mu         sync.RWMutex
	byUpstream map[string]map[string]struct{}
	byRun      map[string][]string
}

func newReverseDependencyGraphIndex() dependencyGraphIndex {
	return &reverseDependencyGraphIndex{
		byUpstream: map[string]map[string]struct{}{},
		byRun:      map[string][]string{},
	}
}

func (i *reverseDependencyGraphIndex) SetRun(run *WorkflowRun) {
	if i == nil || run == nil {
		return
	}
	runID := strings.TrimSpace(run.ID)
	if runID == "" {
		return
	}
	upstreamIDs := dependencyRunIDs(run.Dependencies)
	i.mu.Lock()
	defer i.mu.Unlock()
	i.removeLocked(runID)
	if len(upstreamIDs) == 0 {
		return
	}
	i.byRun[runID] = append([]string(nil), upstreamIDs...)
	for _, upstreamID := range upstreamIDs {
		set := i.byUpstream[upstreamID]
		if set == nil {
			set = map[string]struct{}{}
			i.byUpstream[upstreamID] = set
		}
		set[runID] = struct{}{}
	}
}

func (i *reverseDependencyGraphIndex) RemoveRun(runID string) {
	if i == nil {
		return
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	i.removeLocked(runID)
	delete(i.byUpstream, runID)
}

func (i *reverseDependencyGraphIndex) removeLocked(runID string) {
	upstreamIDs := i.byRun[runID]
	for _, upstreamID := range upstreamIDs {
		dependents := i.byUpstream[upstreamID]
		if dependents == nil {
			continue
		}
		delete(dependents, runID)
		if len(dependents) == 0 {
			delete(i.byUpstream, upstreamID)
		}
	}
	delete(i.byRun, runID)
}

func (i *reverseDependencyGraphIndex) Dependents(runID string) []string {
	if i == nil {
		return nil
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil
	}
	i.mu.RLock()
	defer i.mu.RUnlock()
	set := i.byUpstream[runID]
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for dependentID := range set {
		dependentID = strings.TrimSpace(dependentID)
		if dependentID == "" {
			continue
		}
		out = append(out, dependentID)
	}
	sort.Strings(out)
	return out
}

func (i *reverseDependencyGraphIndex) Reset() {
	if i == nil {
		return
	}
	i.mu.Lock()
	i.byUpstream = map[string]map[string]struct{}{}
	i.byRun = map[string][]string{}
	i.mu.Unlock()
}

type defaultDependencyEvaluator struct{}

func (defaultDependencyEvaluator) Evaluate(
	now time.Time,
	run *WorkflowRun,
	lookup func(runID string) (*WorkflowRun, bool),
) RunDependencyState {
	state := RunDependencyState{}
	if !now.IsZero() {
		nowCopy := now.UTC()
		state.LastEvaluatedAt = &nowCopy
	}
	if run == nil {
		state.Ready = false
		state.Blocking = true
		state.Reason = "workflow run is missing"
		return state
	}
	if len(run.Dependencies) == 0 {
		state.Ready = true
		return state
	}
	unmet := make([]RunDependencySnapshot, 0, len(run.Dependencies))
	blockingReason := ""
	for _, dependency := range run.Dependencies {
		depID := strings.TrimSpace(dependency.RunID)
		condition := dependency.Condition
		if condition == "" {
			condition = DependencyConditionOnCompleted
		}
		snapshot := RunDependencySnapshot{
			RunID:             depID,
			RequiredCondition: condition,
		}
		if depID == "" {
			snapshot.Blocking = true
			snapshot.BlockingReason = "dependency run id is empty"
			unmet = append(unmet, snapshot)
			if blockingReason == "" {
				blockingReason = snapshot.BlockingReason
			}
			continue
		}
		upstream, ok := lookup(depID)
		if !ok || upstream == nil {
			snapshot.Blocking = true
			snapshot.BlockingReason = "dependency run not found: " + depID
			unmet = append(unmet, snapshot)
			if blockingReason == "" {
				blockingReason = snapshot.BlockingReason
			}
			continue
		}
		snapshot.ObservedStatus = upstream.Status
		satisfied, blocked, reason := evaluateDependencyCondition(condition, upstream.Status)
		snapshot.Satisfied = satisfied
		snapshot.Blocking = blocked
		snapshot.BlockingReason = strings.TrimSpace(reason)
		if satisfied {
			continue
		}
		unmet = append(unmet, snapshot)
		if blockingReason == "" && snapshot.Blocking {
			blockingReason = snapshot.BlockingReason
		}
	}
	state.Unmet = unmet
	state.Ready = len(unmet) == 0
	state.Blocking = blockingReason != ""
	if state.Ready {
		state.Reason = ""
		return state
	}
	if blockingReason != "" {
		state.Reason = blockingReason
		return state
	}
	state.Reason = "waiting for dependencies"
	return state
}

func evaluateDependencyCondition(
	condition DependencyCondition,
	upstreamStatus WorkflowRunStatus,
) (satisfied bool, blocked bool, reason string) {
	switch condition {
	case DependencyConditionOnCompleted:
		switch upstreamStatus {
		case WorkflowRunStatusCompleted:
			return true, false, ""
		case WorkflowRunStatusFailed:
			return false, true, "dependency failed"
		case WorkflowRunStatusStopped:
			return false, true, "dependency stopped"
		default:
			return false, false, ""
		}
	default:
		return false, true, "dependency condition invalid"
	}
}

type defaultQueuedRunActivator struct{}

func (defaultQueuedRunActivator) ShouldActivate(run *WorkflowRun, state RunDependencyState) bool {
	return run != nil && run.Status == WorkflowRunStatusQueued && state.Ready
}
