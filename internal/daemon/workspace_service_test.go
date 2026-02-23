package daemon

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"control/internal/types"
)

func TestWorkspaceServiceUpdatePreservesGroupIDsWhenPatchOmitsGroups(t *testing.T) {
	stores := newTestStores(t)
	service := NewWorkspaceService(stores)
	ctx := context.Background()

	repoPath := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}

	created, err := service.Create(ctx, &types.Workspace{RepoPath: repoPath})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	groupIDs := []string{"group-b", "group-a"}
	withGroups, err := service.Update(ctx, created.ID, &types.WorkspacePatch{
		GroupIDs: &groupIDs,
	})
	if err != nil {
		t.Fatalf("set group ids: %v", err)
	}
	if got, want := withGroups.GroupIDs, []string{"group-a", "group-b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected normalized group ids: got=%v want=%v", got, want)
	}

	renamed := "Renamed Workspace"
	updated, err := service.Update(ctx, created.ID, &types.WorkspacePatch{
		Name: &renamed,
	})
	if err != nil {
		t.Fatalf("update workspace name: %v", err)
	}
	if got, want := updated.GroupIDs, withGroups.GroupIDs; !reflect.DeepEqual(got, want) {
		t.Fatalf("expected group ids to be preserved when patch omits groups: got=%v want=%v", got, want)
	}
}

func TestWorkspaceServiceUpdateRejectsInvalidRepoPath(t *testing.T) {
	stores := newTestStores(t)
	ctx := context.Background()

	repoPath := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}

	created, err := NewWorkspaceService(stores).Create(ctx, &types.Workspace{RepoPath: repoPath})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	validateErr := errors.New("validation failed")
	service := NewWorkspaceServiceWithPathResolver(stores, &stubWorkspacePathResolver{validateErr: validateErr})
	nextPath := filepath.Join(t.TempDir(), "repo-next")
	_, err = service.Update(ctx, created.ID, &types.WorkspacePatch{
		RepoPath: &nextPath,
	})
	if err == nil {
		t.Fatalf("expected update to fail")
	}
	if !errors.Is(err, validateErr) {
		t.Fatalf("expected validate error to be wrapped, got %v", err)
	}

	var serviceErr *ServiceError
	if !errors.As(err, &serviceErr) {
		t.Fatalf("expected service error, got %T", err)
	}
	if serviceErr.Kind != ServiceErrorInvalid {
		t.Fatalf("expected invalid service error, got %s", serviceErr.Kind)
	}
}

func TestApplyWorkspacePatchPreservesOmittedFields(t *testing.T) {
	existing := &types.Workspace{
		ID:                    "ws1",
		Name:                  "Workspace",
		RepoPath:              "/tmp/repo",
		SessionSubpath:        "packages/one",
		AdditionalDirectories: []string{"../backend"},
		GroupIDs:              []string{"group-a"},
	}
	name := "  Renamed Workspace  "

	merged, shouldValidate, err := applyWorkspacePatch("ws1", existing, &types.WorkspacePatch{
		Name: &name,
	})
	if err != nil {
		t.Fatalf("apply patch: %v", err)
	}
	if shouldValidate {
		t.Fatalf("expected name-only patch to skip workspace validation")
	}
	if merged.Name != "Renamed Workspace" {
		t.Fatalf("expected trimmed name, got %q", merged.Name)
	}
	if merged.RepoPath != existing.RepoPath {
		t.Fatalf("expected repo path to be preserved, got %q", merged.RepoPath)
	}
	if merged.SessionSubpath != existing.SessionSubpath {
		t.Fatalf("expected session subpath to be preserved, got %q", merged.SessionSubpath)
	}
	if !reflect.DeepEqual(merged.AdditionalDirectories, existing.AdditionalDirectories) {
		t.Fatalf("expected additional directories to be preserved, got=%v want=%v", merged.AdditionalDirectories, existing.AdditionalDirectories)
	}
	if !reflect.DeepEqual(merged.GroupIDs, existing.GroupIDs) {
		t.Fatalf("expected group ids to be preserved, got=%v want=%v", merged.GroupIDs, existing.GroupIDs)
	}
}

func TestApplyWorkspacePatchMarksValidationWhenWorkspacePathsChange(t *testing.T) {
	existing := &types.Workspace{
		ID:                    "ws1",
		Name:                  "Workspace",
		RepoPath:              "/tmp/repo",
		SessionSubpath:        "packages/one",
		AdditionalDirectories: []string{"../backend"},
	}
	repoPath := "/tmp/repo-two"
	sessionSubpath := "packages/two"
	additionalDirs := []string{"../shared"}

	merged, shouldValidate, err := applyWorkspacePatch("ws1", existing, &types.WorkspacePatch{
		RepoPath:              &repoPath,
		SessionSubpath:        &sessionSubpath,
		AdditionalDirectories: &additionalDirs,
	})
	if err != nil {
		t.Fatalf("apply patch: %v", err)
	}
	if !shouldValidate {
		t.Fatalf("expected workspace path changes to require validation")
	}
	if merged.RepoPath != repoPath {
		t.Fatalf("expected repo path %q, got %q", repoPath, merged.RepoPath)
	}
	if merged.SessionSubpath != sessionSubpath {
		t.Fatalf("expected session subpath %q, got %q", sessionSubpath, merged.SessionSubpath)
	}
	if !reflect.DeepEqual(merged.AdditionalDirectories, additionalDirs) {
		t.Fatalf("expected additional directories %v, got %v", additionalDirs, merged.AdditionalDirectories)
	}
}

func TestValidateWorkspaceUpdateSkipsValidationWhenNotRequired(t *testing.T) {
	if err := validateWorkspaceUpdate(nil, false, nil); err != nil {
		t.Fatalf("expected no validation error when validation is skipped, got %v", err)
	}
}

func TestApplyWorkspacePatchRejectsNilExisting(t *testing.T) {
	_, _, err := applyWorkspacePatch("ws1", nil, &types.WorkspacePatch{})
	if err == nil {
		t.Fatalf("expected missing existing workspace error")
	}
	if !strings.Contains(err.Error(), "workspace not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestApplyWorkspacePatchRejectsNilPatch(t *testing.T) {
	existing := &types.Workspace{
		ID:       "ws1",
		RepoPath: "/tmp/repo",
	}
	_, _, err := applyWorkspacePatch("ws1", existing, nil)
	if err == nil {
		t.Fatalf("expected nil patch error")
	}
	if !strings.Contains(err.Error(), "workspace payload is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateWorkspaceUpdateRequiresWorkspaceWhenValidationNeeded(t *testing.T) {
	err := validateWorkspaceUpdate(nil, true, &countingWorkspaceValidator{})
	if err == nil {
		t.Fatalf("expected missing workspace to fail")
	}
	if !strings.Contains(err.Error(), "workspace is required") {
		t.Fatalf("unexpected missing workspace error: %v", err)
	}
}

func TestValidateWorkspaceUpdateRequiresValidatorWhenValidationNeeded(t *testing.T) {
	repoPath := t.TempDir()
	err := validateWorkspaceUpdate(&types.Workspace{RepoPath: repoPath}, true, nil)
	if err == nil {
		t.Fatalf("expected missing validator to fail")
	}
	if !strings.Contains(err.Error(), "workspace validator is required") {
		t.Fatalf("unexpected missing validator error: %v", err)
	}
}

func TestValidateWorkspaceUpdateInvokesWorkspaceValidator(t *testing.T) {
	repoPath := t.TempDir()
	validator := &countingWorkspaceValidator{}
	if err := validateWorkspaceUpdate(&types.Workspace{RepoPath: repoPath}, true, validator); err != nil {
		t.Fatalf("expected validation to pass, got %v", err)
	}
	if validator.calls != 1 {
		t.Fatalf("expected validator to be called once, got %d", validator.calls)
	}
}

func TestValidateWorkspaceUpdatePropagatesWorkspaceValidatorError(t *testing.T) {
	repoPath := t.TempDir()
	validateErr := errors.New("validator boom")
	validator := &countingWorkspaceValidator{err: validateErr}
	err := validateWorkspaceUpdate(&types.Workspace{RepoPath: repoPath}, true, validator)
	if err == nil {
		t.Fatalf("expected validation error")
	}
	if !errors.Is(err, validateErr) {
		t.Fatalf("expected validator error to propagate, got %v", err)
	}
}

func TestValidateWorkspaceUpdatePropagatesAdditionalDirectoryValidationError(t *testing.T) {
	repoPath := t.TempDir()
	additionalDirectories := []string{"./missing"}
	err := validateWorkspaceUpdate(&types.Workspace{
		RepoPath:              repoPath,
		AdditionalDirectories: additionalDirectories,
	}, true, &countingWorkspaceValidator{})
	if err == nil {
		t.Fatalf("expected additional directories validation error")
	}
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if !strings.Contains(strings.ToLower(err.Error()), "no such file") {
		t.Fatalf("expected filesystem validation error, got %v", err)
	}
}

type countingWorkspaceValidator struct {
	calls int
	err   error
}

func (v *countingWorkspaceValidator) ValidateWorkspace(_, _ string) error {
	v.calls++
	return v.err
}
