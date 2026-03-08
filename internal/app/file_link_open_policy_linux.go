package app

import "strings"

type linuxFileLinkOpenPolicy struct{}

func currentPlatformFileLinkOpenPolicy() FileLinkOpenPolicy {
	return linuxFileLinkOpenPolicy{}
}

func (linuxFileLinkOpenPolicy) BuildCommand(target ResolvedFileLink) (FileLinkOpenCommand, error) {
	openTarget := strings.TrimSpace(target.OpenTarget())
	if openTarget == "" {
		return FileLinkOpenCommand{}, errFileLinkEmptyTarget
	}
	return FileLinkOpenCommand{Name: "xdg-open", Args: []string{openTarget}}, nil
}
