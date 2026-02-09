package main

import (
	"context"
	"flag"
	"io"
)

type PSCommand struct {
	stdout    io.Writer
	stderr    io.Writer
	newClient clientFactory
}

func NewPSCommand(stdout, stderr io.Writer, newClient clientFactory) *PSCommand {
	return &PSCommand{
		stdout:    stdout,
		stderr:    stderr,
		newClient: newClient,
	}
}

func (c *PSCommand) Run(args []string) error {
	fs := flag.NewFlagSet("ps", flag.ContinueOnError)
	fs.SetOutput(c.stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx := context.Background()
	client, err := c.newClient()
	if err != nil {
		return err
	}
	if err := client.EnsureDaemon(ctx); err != nil {
		return err
	}
	sessions, err := client.ListSessions(ctx)
	if err != nil {
		return err
	}

	printSessions(c.stdout, sessions)
	return nil
}
