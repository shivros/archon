package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
)

type KillCommand struct {
	stdout    io.Writer
	stderr    io.Writer
	newClient clientFactory
}

func NewKillCommand(stdout, stderr io.Writer, newClient clientFactory) *KillCommand {
	return &KillCommand{
		stdout:    stdout,
		stderr:    stderr,
		newClient: newClient,
	}
}

func (c *KillCommand) Run(args []string) error {
	fs := flag.NewFlagSet("kill", flag.ContinueOnError)
	fs.SetOutput(c.stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return errors.New("kill requires a session id")
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
	if err := client.KillSession(ctx, id); err != nil {
		return err
	}
	fmt.Fprintln(c.stdout, "ok")
	return nil
}
