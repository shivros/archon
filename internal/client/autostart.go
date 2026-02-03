package client

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"control/internal/config"
)

func StartBackgroundDaemon() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}

	cmd := exec.Command(exe, "daemon", "--background")
	applyDaemonSysProcAttr(cmd)

	logWriter := io.Discard
	var logFile *os.File
	if dataDir, err := config.DataDir(); err == nil {
		if err := os.MkdirAll(dataDir, 0o700); err == nil {
			logPath := filepath.Join(dataDir, "daemon.log")
			if file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
				logWriter = file
				logFile = file
			}
		}
	}
	cmd.Stdout = logWriter
	cmd.Stderr = logWriter

	err = cmd.Start()
	if logFile != nil {
		_ = logFile.Close()
	}
	return err
}
