package main

import (
	"context"
	"flag"
	"io"
	"log"
	"os"
	"path/filepath"

	"control/internal/config"
)

type UICommand struct {
	stderr             io.Writer
	newClient          clientFactory
	configureUILogging func()
	version            string
}

func NewUICommand(stderr io.Writer, newClient clientFactory, configureUILogging func(), version string) *UICommand {
	return &UICommand{
		stderr:             stderr,
		newClient:          newClient,
		configureUILogging: configureUILogging,
		version:            version,
	}
}

func (c *UICommand) Run(args []string) error {
	fs := flag.NewFlagSet("ui", flag.ContinueOnError)
	fs.SetOutput(c.stderr)
	restartDaemon := fs.Bool("restart-daemon", false, "restart daemon if version mismatch")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if c.configureUILogging != nil {
		c.configureUILogging()
	}

	ctx := context.Background()
	client, err := c.newClient()
	if err != nil {
		return err
	}
	if err := client.EnsureDaemonVersion(ctx, c.version, *restartDaemon); err != nil {
		return err
	}
	return client.RunUI()
}

func configureUILogging() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	dataDir, err := config.DataDir()
	if err != nil {
		return
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return
	}
	logPath := filepath.Join(dataDir, "ui.log")
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	log.SetOutput(file)
}
