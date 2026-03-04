package app

import (
	"strings"
)

type defaultFileLinkOpenPolicy struct{}

func newDefaultFileLinkOpenPolicy() FileLinkOpenPolicy {
	return defaultFileLinkOpenPolicy{}
}

func (defaultFileLinkOpenPolicy) BuildCommand(target ResolvedFileLink) (FileLinkOpenCommand, error) {
	path := strings.TrimSpace(target.Path)
	if path == "" {
		return FileLinkOpenCommand{}, errFileLinkEmptyTarget
	}
	policy := currentPlatformFileLinkOpenPolicy()
	return policy.BuildCommand(target)
}
