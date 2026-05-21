package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

type LogoutCommand struct {
	stdout    io.Writer
	stderr    io.Writer
	newClient cloudAuthClientFactory
}

func NewLogoutCommand(stdout, stderr io.Writer, newClient cloudAuthClientFactory) *LogoutCommand {
	return &LogoutCommand{
		stdout:    stdout,
		stderr:    stderr,
		newClient: newClient,
	}
}

func (c *LogoutCommand) Run(args []string) error {
	fs := flag.NewFlagSet("logout", flag.ContinueOnError)
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

	resp, err := client.LogoutCloud(ctx)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintln(c.stdout, resp.Message)
	return nil
}
