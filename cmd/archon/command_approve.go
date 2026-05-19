package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	controlclient "control/internal/client"
)

type ApproveCommand struct {
	stdout    io.Writer
	stderr    io.Writer
	newClient sessionClientFactory
}

func NewApproveCommand(stdout, stderr io.Writer, newClient sessionClientFactory) *ApproveCommand {
	return &ApproveCommand{
		stdout:    stdout,
		stderr:    stderr,
		newClient: newClient,
	}
}

func (c *ApproveCommand) Run(args []string) error {
	args = reorderApproveFlags(args)
	fs := flag.NewFlagSet("approve", flag.ContinueOnError)
	fs.SetOutput(c.stderr)
	requestID := fs.Int("request-id", 0, "approval request id (required)")
	decision := fs.String("decision", "", "approval decision (required)")
	var responses stringList
	fs.Var(&responses, "response", "response string (repeatable)")
	acceptSettings := fs.String("accept-settings", "", "JSON object for accept_settings (optional)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return errors.New("approve requires a session id")
	}
	sessionID := fs.Arg(0)

	if *requestID == 0 {
		return errors.New("--request-id is required")
	}
	if *decision == "" {
		return errors.New("--decision is required")
	}

	var acceptSettingsMap map[string]any
	if *acceptSettings != "" {
		if err := json.Unmarshal([]byte(*acceptSettings), &acceptSettingsMap); err != nil {
			return fmt.Errorf("invalid --accept-settings JSON: %v", err)
		}
	}

	req := controlclient.ApproveSessionRequest{
		RequestID:      *requestID,
		Decision:       *decision,
		AcceptSettings: acceptSettingsMap,
	}
	if len(responses) > 0 {
		req.Responses = []string(responses)
	}

	ctx := context.Background()
	client, err := c.newClient()
	if err != nil {
		return err
	}
	if err := client.EnsureDaemon(ctx); err != nil {
		return err
	}
	if err := client.ApproveSession(ctx, sessionID, req); err != nil {
		return err
	}

	return nil
}

// reorderApproveFlags moves all flags (with their values) before positional
// arguments. This works around Go's flag package stopping at the first
// non-flag argument. Known value-taking flags: request-id, decision, response,
// accept-settings.
func reorderApproveFlags(args []string) []string {
	valueFlags := map[string]bool{
		"request-id":      true,
		"decision":        true,
		"response":        true,
		"accept-settings": true,
	}
	var flags, positionals []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "--") {
			name := strings.TrimPrefix(a, "--")
			if idx := strings.IndexByte(name, '='); idx >= 0 {
				flags = append(flags, a)
				continue
			}
			flags = append(flags, a)
			if valueFlags[name] && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
		} else {
			positionals = append(positionals, a)
		}
	}
	return append(flags, positionals...)
}
