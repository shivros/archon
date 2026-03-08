package app

import (
	"strings"
)

type defaultFileLinkOpenPolicy struct{}

func newDefaultFileLinkOpenPolicy() FileLinkOpenPolicy {
	return defaultFileLinkOpenPolicy{}
}

func (defaultFileLinkOpenPolicy) BuildCommand(target ResolvedFileLink) (FileLinkOpenCommand, error) {
	openTarget := strings.TrimSpace(target.OpenTarget())
	if openTarget == "" {
		return FileLinkOpenCommand{}, errFileLinkEmptyTarget
	}
	policy := currentPlatformFileLinkOpenPolicy()
	return policy.BuildCommand(target)
}
