//go:build windows

package client

import "os/exec"

func applyDaemonSysProcAttr(cmd *exec.Cmd) {}
