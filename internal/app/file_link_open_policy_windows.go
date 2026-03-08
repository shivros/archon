package app

import "strings"

type windowsFileLinkOpenPolicy struct{}

func currentPlatformFileLinkOpenPolicy() FileLinkOpenPolicy {
	return windowsFileLinkOpenPolicy{}
}

func (windowsFileLinkOpenPolicy) BuildCommand(target ResolvedFileLink) (FileLinkOpenCommand, error) {
	openTarget := strings.TrimSpace(target.OpenTarget())
	if openTarget == "" {
		return FileLinkOpenCommand{}, errFileLinkEmptyTarget
	}
	return FileLinkOpenCommand{Name: "cmd", Args: []string{"/c", "start", "", openTarget}}, nil
}
