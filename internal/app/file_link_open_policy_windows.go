package app

import "strings"

type windowsFileLinkOpenPolicy struct{}

func currentPlatformFileLinkOpenPolicy() FileLinkOpenPolicy {
	return windowsFileLinkOpenPolicy{}
}

func (windowsFileLinkOpenPolicy) BuildCommand(target ResolvedFileLink) (FileLinkOpenCommand, error) {
	path := strings.TrimSpace(target.Path)
	if path == "" {
		return FileLinkOpenCommand{}, errFileLinkEmptyTarget
	}
	return FileLinkOpenCommand{Name: "cmd", Args: []string{"/c", "start", "", path}}, nil
}
