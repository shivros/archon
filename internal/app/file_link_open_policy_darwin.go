package app

import "strings"

type darwinFileLinkOpenPolicy struct{}

func currentPlatformFileLinkOpenPolicy() FileLinkOpenPolicy {
	return darwinFileLinkOpenPolicy{}
}

func (darwinFileLinkOpenPolicy) BuildCommand(target ResolvedFileLink) (FileLinkOpenCommand, error) {
	path := strings.TrimSpace(target.Path)
	if path == "" {
		return FileLinkOpenCommand{}, errFileLinkEmptyTarget
	}
	return FileLinkOpenCommand{Name: "open", Args: []string{path}}, nil
}
