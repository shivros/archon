package app

import "strings"

type darwinFileLinkOpenPolicy struct{}

func currentPlatformFileLinkOpenPolicy() FileLinkOpenPolicy {
	return darwinFileLinkOpenPolicy{}
}

func (darwinFileLinkOpenPolicy) BuildCommand(target ResolvedFileLink) (FileLinkOpenCommand, error) {
	openTarget := strings.TrimSpace(target.OpenTarget())
	if openTarget == "" {
		return FileLinkOpenCommand{}, errFileLinkEmptyTarget
	}
	return FileLinkOpenCommand{Name: "open", Args: []string{openTarget}}, nil
}
