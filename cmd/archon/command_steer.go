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

// SteerCommand sends a steering message to an already-running turn in an
// existing session. Unlike send (which starts a new turn), steer injects
// additional instructions into the current in-flight turn.
type SteerCommand struct {
	stdout    io.Writer
	stderr    io.Writer
	stdin     io.Reader
	newClient sessionClientFactory
}

// NewSteerCommand creates a new steer command.
func NewSteerCommand(stdout, stderr io.Writer, stdin io.Reader, newClient sessionClientFactory) *SteerCommand {
	return &SteerCommand{
		stdout:    stdout,
		stderr:    stderr,
		stdin:     stdin,
		newClient: newClient,
	}
}

func (c *SteerCommand) Run(args []string) error {
	// Reorder flags before positionals so Go's flag parser sees them.
	reordered := reorderSteerFlags(args)

	fs := flag.NewFlagSet("steer", flag.ContinueOnError)
	fs.SetOutput(c.stderr)
	textFlag := fs.String("text", "", "steering text (mutually exclusive with positional text)")
	jsonOutput := fs.Bool("json", false, "print full response as JSON instead of just the turn id")
	if err := fs.Parse(reordered); err != nil {
		return err
	}

	// Collect positional args (everything after flags).
	var positionalArgs []string
	for _, a := range fs.Args() {
		positionalArgs = append(positionalArgs, a)
	}

	// Must have at least a session ID.
	if len(positionalArgs) < 1 {
		return errors.New("steer requires a session id")
	}
	sessionID := positionalArgs[0]

	// Determine input form: exactly one of positional text (2nd arg) or --text.
	positionalText := ""
	if len(positionalArgs) >= 2 {
		positionalText = positionalArgs[1]
	}

	inputForms := 0
	if positionalText != "" {
		inputForms++
	}
	if *textFlag != "" {
		inputForms++
	}

	switch inputForms {
	case 0:
		return errors.New("steer requires a message: provide positional text or --text")
	case 1:
		// ok
	default:
		return errors.New("provide exactly one of: positional text or --text")
	}

	// Build the request.
	var req controlclient.SteerSessionRequest
	text := positionalText
	if text == "" {
		text = *textFlag
	}
	req.Text = text

	// Contact daemon.
	ctx := context.Background()
	client, err := c.newClient()
	if err != nil {
		return err
	}
	if err := client.EnsureDaemon(ctx); err != nil {
		return err
	}

	resp, err := client.SteerSession(ctx, sessionID, req)
	if err != nil {
		return err
	}

	// Output.
	if *jsonOutput {
		enc := json.NewEncoder(c.stdout)
		enc.SetEscapeHTML(false)
		return enc.Encode(resp)
	}
	// Default: print turn_id if present.
	if resp.TurnID != "" {
		fmt.Fprintln(c.stdout, resp.TurnID)
	}
	return nil
}

// reorderSteerFlags moves all flags (with their values) before positional
// arguments. This works around Go's flag package stopping at the first
// non-flag argument. Known value-taking flags: text. Bool flags: json.
func reorderSteerFlags(args []string) []string {
	valueFlags := map[string]bool{
		"text": true,
	}
	var flags, positionals []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "--") {
			name := strings.TrimPrefix(a, "--")
			// Handle --flag=value form.
			if idx := strings.IndexByte(name, '='); idx >= 0 {
				flags = append(flags, a)
				continue
			}
			flags = append(flags, a)
			if valueFlags[name] && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
		} else if strings.HasPrefix(a, "-") && len(a) == 2 {
			flags = append(flags, a)
		} else {
			positionals = append(positionals, a)
		}
	}
	return append(flags, positionals...)
}
