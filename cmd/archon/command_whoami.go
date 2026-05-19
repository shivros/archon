package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

type WhoAmICommand struct {
	stdout    io.Writer
	stderr    io.Writer
	newClient cloudAuthClientFactory
}

func NewWhoAmICommand(stdout, stderr io.Writer, newClient cloudAuthClientFactory) *WhoAmICommand {
	return &WhoAmICommand{
		stdout:    stdout,
		stderr:    stderr,
		newClient: newClient,
	}
}

func (c *WhoAmICommand) Run(args []string) error {
	fs := flag.NewFlagSet("whoami", flag.ContinueOnError)
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

	status, err := client.CloudAuthStatus(ctx)
	if err != nil {
		return err
	}

	if !status.Linked {
		_, _ = fmt.Fprintln(c.stdout, "not logged in")
		return nil
	}

	if status.User != nil {
		if status.User.DisplayName != "" {
			_, _ = fmt.Fprintf(c.stdout, "User: %s\n", status.User.DisplayName)
		}
		if status.User.Email != "" {
			_, _ = fmt.Fprintf(c.stdout, "Email: %s\n", status.User.Email)
		}
	}
	if status.Installation != nil && status.Installation.Name != "" {
		_, _ = fmt.Fprintf(c.stdout, "Installation: %s\n", status.Installation.Name)
	}

	return nil
}
