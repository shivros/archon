package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	controlclient "control/internal/client"
)

// SendCommand sends a message to an existing session.
type SendCommand struct {
	stdout    io.Writer
	stderr    io.Writer
	stdin     io.Reader
	newClient sessionClientFactory
}

// NewSendCommand creates a new send command.
func NewSendCommand(stdout, stderr io.Writer, stdin io.Reader, newClient sessionClientFactory) *SendCommand {
	return &SendCommand{
		stdout:    stdout,
		stderr:    stderr,
		stdin:     stdin,
		newClient: newClient,
	}
}

func (c *SendCommand) Run(args []string) error {
	// Reorder flags before positionals so Go's flag parser sees them.
	reordered := reorderSendFlags(args)

	fs := flag.NewFlagSet("send", flag.ContinueOnError)
	fs.SetOutput(c.stderr)
	textFlag := fs.String("text", "", "message text (mutually exclusive with positional text and --input-items)")
	inputItemsFlag := fs.String("input-items", "", "path to JSON file containing input array, or - for stdin (mutually exclusive with --text and positional text)")
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
		return errors.New("send requires a session id")
	}
	sessionID := positionalArgs[0]

	// Determine input form: exactly one of positional text (2nd arg), --text, --input-items.
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
	if *inputItemsFlag != "" {
		inputForms++
	}

	switch inputForms {
	case 0:
		return errors.New("send requires a message: provide positional text, --text, or --input-items")
	case 1:
		// ok
	default:
		return errors.New("provide exactly one of: positional text, --text, or --input-items")
	}

	// Build the request.
	var req controlclient.SendSessionRequest
	if *inputItemsFlag != "" {
		var raw []byte
		var err error
		if *inputItemsFlag == "-" {
			raw, err = io.ReadAll(c.stdin)
		} else {
			raw, err = os.ReadFile(*inputItemsFlag)
		}
		if err != nil {
			return fmt.Errorf("reading input items: %v", err)
		}
		var items []map[string]any
		if err := json.Unmarshal(raw, &items); err != nil {
			return fmt.Errorf("parsing input items JSON: %v", err)
		}
		req.Input = items
	} else {
		text := positionalText
		if text == "" {
			text = *textFlag
		}
		req.Text = text
	}

	// Contact daemon.
	ctx := context.Background()
	client, err := c.newClient()
	if err != nil {
		return err
	}
	if err := client.EnsureDaemon(ctx); err != nil {
		return err
	}

	resp, err := client.SendMessage(ctx, sessionID, req)
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

// reorderSendFlags moves all flags (with their values) before positional
// arguments. This works around Go's flag package stopping at the first
// non-flag argument. Known value-taking flags: text, input-items. Bool
// flags: json.
func reorderSendFlags(args []string) []string {
	valueFlags := map[string]bool{
		"text":        true,
		"input-items": true,
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
			// Short flags not used by send currently, but handle gracefully.
			flags = append(flags, a)
		} else {
			positionals = append(positionals, a)
		}
	}
	return append(flags, positionals...)
}
