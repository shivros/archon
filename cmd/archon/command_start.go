package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	controlclient "control/internal/client"
)

type StartCommand struct {
	stdout    io.Writer
	stderr    io.Writer
	newClient clientFactory
}

func NewStartCommand(stdout, stderr io.Writer, newClient clientFactory) *StartCommand {
	return &StartCommand{
		stdout:    stdout,
		stderr:    stderr,
		newClient: newClient,
	}
}

func (c *StartCommand) Run(args []string) error {
	fs := flag.NewFlagSet("start", flag.ContinueOnError)
	fs.SetOutput(c.stderr)
	provider := fs.String("provider", "", "provider name")
	cwd := fs.String("cwd", "", "working directory")
	cmd := fs.String("cmd", "", "command override (custom provider)")
	title := fs.String("title", "", "session title")
	var tags stringList
	var envs stringList
	fs.Var(&tags, "tag", "tag (repeatable)")
	fs.Var(&envs, "env", "environment variable KEY=VALUE (repeatable)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cmdArgs := fs.Args()
	if *provider == "" {
		return errors.New("provider is required")
	}

	ctx := context.Background()
	client, err := c.newClient()
	if err != nil {
		return err
	}
	if err := client.EnsureDaemon(ctx); err != nil {
		return err
	}

	session, err := client.StartSession(ctx, controlclient.StartSessionRequest{
		Provider: *provider,
		Cmd:      *cmd,
		Cwd:      *cwd,
		Args:     cmdArgs,
		Env:      envs,
		Title:    *title,
		Tags:     tags,
	})
	if err != nil {
		return err
	}
	fmt.Fprintln(c.stdout, session.ID)
	return nil
}
