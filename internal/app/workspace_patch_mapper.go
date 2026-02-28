package app

import (
	"strings"

	"control/internal/types"
)

type workspaceFormData struct {
	Path                     string
	SessionSubpath           string
	AdditionalDirectoriesRaw string
	Name                     string
	GroupIDs                 []string
}

func workspacePatchFromForm(form workspaceFormData) *types.WorkspacePatch {
	trimmedPath := strings.TrimSpace(form.Path)
	trimmedSubpath := strings.TrimSpace(form.SessionSubpath)
	trimmedName := strings.TrimSpace(form.Name)
	directories := parseAdditionalDirectories(form.AdditionalDirectoriesRaw)
	if directories == nil {
		directories = []string{}
	}
	gids := append([]string(nil), form.GroupIDs...)
	if gids == nil {
		gids = []string{}
	}

	return &types.WorkspacePatch{
		Name:                  &trimmedName,
		RepoPath:              &trimmedPath,
		SessionSubpath:        &trimmedSubpath,
		AdditionalDirectories: &directories,
		GroupIDs:              &gids,
	}
}
