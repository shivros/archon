//go:build !windows

package client

import (
	"os/exec"
	"syscall"
)

func applyDaemonSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}
}
