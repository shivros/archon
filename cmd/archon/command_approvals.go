package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"control/internal/types"
)

type ApprovalsCommand struct {
	stdout    io.Writer
	stderr    io.Writer
	newClient sessionClientFactory
}

func NewApprovalsCommand(stdout, stderr io.Writer, newClient sessionClientFactory) *ApprovalsCommand {
	return &ApprovalsCommand{
		stdout:    stdout,
		stderr:    stderr,
		newClient: newClient,
	}
}

func (c *ApprovalsCommand) Run(args []string) error {
	fs := flag.NewFlagSet("approvals", flag.ContinueOnError)
	fs.SetOutput(c.stderr)
	emitJSON := fs.Bool("json", false, "emit machine-readable JSON array of approvals")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return errors.New("approvals requires a session id")
	}
	sessionID := fs.Arg(0)

	ctx := context.Background()
	client, err := c.newClient()
	if err != nil {
		return err
	}
	if err := client.EnsureDaemon(ctx); err != nil {
		return err
	}
	approvals, err := client.ListApprovals(ctx, sessionID)
	if err != nil {
		return err
	}

	if *emitJSON {
		if approvals == nil {
			approvals = []*types.Approval{}
		}
		encoded, err := json.MarshalIndent(approvals, "", "  ")
		if err != nil {
			return err
		}
		if _, err := c.stdout.Write(encoded); err != nil {
			return err
		}
		_, err = fmt.Fprint(c.stdout, "\n")
		return err
	}

	printApprovals(c.stdout, approvals)
	return nil
}

func printApprovals(output io.Writer, approvals []*types.Approval) {
	writer := tabwriter.NewWriter(output, 0, 8, 2, ' ', 0)
	_, _ = fmt.Fprintln(writer, "REQUEST_ID\tMETHOD\tCREATED")
	for _, a := range approvals {
		created := a.CreatedAt.Local().Format("2006-01-02 15:04:05")
		_, _ = fmt.Fprintf(writer, "%d\t%s\t%s\n", a.RequestID, a.Method, created)
	}
	_ = writer.Flush()
}

// approvalsTimeNow is overridden in tests for deterministic timestamps.
var approvalsTimeNow = time.Now
