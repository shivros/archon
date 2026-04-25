package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
)

type PSCommand struct {
	stdout    io.Writer
	stderr    io.Writer
	newClient sessionClientFactory
}

func NewPSCommand(stdout, stderr io.Writer, newClient sessionClientFactory) *PSCommand {
	return &PSCommand{
		stdout:    stdout,
		stderr:    stderr,
		newClient: newClient,
	}
}

func (c *PSCommand) Run(args []string) error {
	fs := flag.NewFlagSet("ps", flag.ContinueOnError)
	fs.SetOutput(c.stderr)
	emitJSON := fs.Bool("json", false, "emit machine-readable JSON array of sessions")
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

	if *emitJSON {
		if len(sessions) == 0 {
			_, err := fmt.Fprint(c.stdout, "[]\n")
			return err
		}
		encoded, err := json.MarshalIndent(sessions, "", "  ")
		if err != nil {
			return err
		}
		if _, err := c.stdout.Write(encoded); err != nil {
			return err
		}
		_, err = fmt.Fprint(c.stdout, "\n")
		return err
	}

	printSessions(c.stdout, sessions)
	return nil
}
