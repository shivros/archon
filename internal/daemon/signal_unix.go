//go:build !windows

package daemon

import (
	"os"
	"syscall"
)

func signalTerminate(process *os.Process) error {
	return process.Signal(syscall.SIGTERM)
}

func signalKill(process *os.Process) error {
	return process.Signal(syscall.SIGKILL)
}
