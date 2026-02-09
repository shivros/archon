package main

import (
	"io"
	"os"
)

type commandRunner interface {
	Run(args []string) error
}

type commandWiring struct {
	stdout             io.Writer
	stderr             io.Writer
	newClient          clientFactory
	runDaemon          func(background bool) error
	killDaemon         func() error
	configureUILogging func()
	version            string
}

func defaultCommandWiring(stdout, stderr io.Writer) commandWiring {
	if stdout == nil {
		stdout = os.Stdout
	}
	if stderr == nil {
		stderr = os.Stderr
	}
	return commandWiring{
		stdout:    stdout,
		stderr:    stderr,
		newClient: newControlClient,
		runDaemon: runDaemonProcess,
		killDaemon: func() error {
			return killDaemonWithFactory(newControlClient)
		},
		configureUILogging: configureUILogging,
		version:            buildVersion(),
	}
}

func buildCommands(wiring commandWiring) map[string]commandRunner {
	return map[string]commandRunner{
		"daemon": NewDaemonCommand(wiring.stderr, wiring.runDaemon, wiring.killDaemon),
		"ps":     NewPSCommand(wiring.stdout, wiring.stderr, wiring.newClient),
		"start":  NewStartCommand(wiring.stdout, wiring.stderr, wiring.newClient),
		"kill":   NewKillCommand(wiring.stdout, wiring.stderr, wiring.newClient),
		"tail":   NewTailCommand(wiring.stdout, wiring.stderr, wiring.newClient),
		"ui":     NewUICommand(wiring.stderr, wiring.newClient, wiring.configureUILogging, wiring.version),
	}
}
