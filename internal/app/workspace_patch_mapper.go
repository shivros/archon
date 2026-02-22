package app

import (
	"strings"

	"control/internal/types"
)

func workspacePatchFromForm(path, sessionSubpath, additionalDirectoriesRaw, name string) *types.WorkspacePatch {
	trimmedPath := strings.TrimSpace(path)
	trimmedSubpath := strings.TrimSpace(sessionSubpath)
	trimmedName := strings.TrimSpace(name)
	directories := parseAdditionalDirectories(additionalDirectoriesRaw)
	if directories == nil {
		directories = []string{}
	}

	return &types.WorkspacePatch{
		Name:                  &trimmedName,
		RepoPath:              &trimmedPath,
		SessionSubpath:        &trimmedSubpath,
		AdditionalDirectories: &directories,
	}
}
