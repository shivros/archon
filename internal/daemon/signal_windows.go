//go:build windows

package daemon

import "os"

func signalTerminate(process *os.Process) error {
	return process.Kill()
}

func signalKill(process *os.Process) error {
	return process.Kill()
}
