package app

import "strings"

type linuxFileLinkOpenPolicy struct{}

func currentPlatformFileLinkOpenPolicy() FileLinkOpenPolicy {
	return linuxFileLinkOpenPolicy{}
}

func (linuxFileLinkOpenPolicy) BuildCommand(target ResolvedFileLink) (FileLinkOpenCommand, error) {
	path := strings.TrimSpace(target.Path)
	if path == "" {
		return FileLinkOpenCommand{}, errFileLinkEmptyTarget
	}
	return FileLinkOpenCommand{Name: "xdg-open", Args: []string{path}}, nil
}
