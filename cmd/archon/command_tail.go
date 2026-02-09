package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
)

type TailCommand struct {
	stdout    io.Writer
	stderr    io.Writer
	newClient clientFactory
}

func NewTailCommand(stdout, stderr io.Writer, newClient clientFactory) *TailCommand {
	return &TailCommand{
		stdout:    stdout,
		stderr:    stderr,
		newClient: newClient,
	}
}

func (c *TailCommand) Run(args []string) error {
	fs := flag.NewFlagSet("tail", flag.ContinueOnError)
	fs.SetOutput(c.stderr)
	lines := fs.Int("lines", 200, "number of lines to fetch")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return errors.New("tail requires a session id")
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
	resp, err := client.TailItems(ctx, id, *lines)
	if err != nil {
		return err
	}
	return json.NewEncoder(c.stdout).Encode(resp.Items)
}
