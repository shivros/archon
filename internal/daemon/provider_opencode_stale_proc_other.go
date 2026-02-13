//go:build !linux

package daemon

import "errors"

func findOpenCodeServerPIDImpl(_ string, _ string) (int, error) {
	return 0, errors.New("stale process lookup is only supported on linux")
}

func readOpenCodeProcessCmdlineImpl(_ int) (string, error) {
	return "", errors.New("process cmdline lookup is only supported on linux")
}
