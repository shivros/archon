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
	newCloudAuthClient cloudAuthClientFactory
	newSessionClient   sessionClientFactory
	newUIClient        daemonVersionClientFactory
	newDaemonAdmin     daemonAdminClientFactory
	openBrowser        browserOpener
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
		stdout:             stdout,
		stderr:             stderr,
		newCloudAuthClient: newCloudAuthClient,
		newSessionClient:   newSessionClient,
		newUIClient:        newDaemonVersionClient,
		newDaemonAdmin:     newDaemonAdminClient,
		openBrowser:        openBrowserURL,
		runDaemon:          runDaemonProcess,
		killDaemon: func() error {
			return killDaemonWithFactory(newDaemonAdminClient)
		},
		configureUILogging: configureUILogging,
		version:            buildVersion(),
	}
}

func buildCommands(wiring commandWiring) map[string]commandRunner {
	return map[string]commandRunner{
		"daemon":  NewDaemonCommand(wiring.stderr, wiring.runDaemon, wiring.killDaemon),
		"config":  NewConfigCommand(wiring.stdout, wiring.stderr),
		"login":   NewLoginCommand(wiring.stdout, wiring.stderr, wiring.newCloudAuthClient, wiring.openBrowser),
		"whoami":  NewWhoAmICommand(wiring.stdout, wiring.stderr, wiring.newCloudAuthClient),
		"logout":  NewLogoutCommand(wiring.stdout, wiring.stderr, wiring.newCloudAuthClient),
		"ps":      NewPSCommand(wiring.stdout, wiring.stderr, wiring.newSessionClient),
		"start":   NewStartCommand(wiring.stdout, wiring.stderr, wiring.newSessionClient),
		"kill":    NewKillCommand(wiring.stdout, wiring.stderr, wiring.newSessionClient),
		"tail":    NewTailCommand(wiring.stdout, wiring.stderr, wiring.newSessionClient),
		"ui":      NewUICommand(wiring.stderr, wiring.newUIClient, wiring.configureUILogging, wiring.version),
		"version": NewVersionCommand(wiring.stdout, wiring.stderr),
	}
}
