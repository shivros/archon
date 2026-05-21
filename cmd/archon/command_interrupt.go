package main

import (
	"context"
	"errors"
	"flag"
	"io"
)

type InterruptCommand struct {
	stdout    io.Writer
	stderr    io.Writer
	newClient sessionClientFactory
}

func NewInterruptCommand(stdout, stderr io.Writer, newClient sessionClientFactory) *InterruptCommand {
	return &InterruptCommand{
		stdout:    stdout,
		stderr:    stderr,
		newClient: newClient,
	}
}

func (c *InterruptCommand) Run(args []string) error {
	fs := flag.NewFlagSet("interrupt", flag.ContinueOnError)
	fs.SetOutput(c.stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return errors.New("interrupt requires a session id")
	}
	id := fs.Arg(0)

	ctx := context.Background()
	client, err := c.newClient()
	if err != nil {
		return err
	}
	if err := client.EnsureDaemon(ctx); err != nil {
		return err
	}
	if err := client.InterruptSession(ctx, id); err != nil {
		return err
	}
	return nil
}
