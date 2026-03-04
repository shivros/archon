//go:build !linux && !darwin && !windows

package app

import (
	"fmt"
	"runtime"
)

type unsupportedFileLinkOpenPolicy struct{}

func currentPlatformFileLinkOpenPolicy() FileLinkOpenPolicy {
	return unsupportedFileLinkOpenPolicy{}
}

func (unsupportedFileLinkOpenPolicy) BuildCommand(ResolvedFileLink) (FileLinkOpenCommand, error) {
	return FileLinkOpenCommand{}, fmt.Errorf("unsupported platform: %s", runtime.GOOS)
}
