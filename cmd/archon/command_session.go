package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"control/internal/types"
)

const sessionHelpText = `Usage: archon session <id> [flags]

Print full details for a single session.

Default output is pretty-printed JSON (same shape as each element in ps --json).
Use --format human for a compact field-per-line view.

Flags:
  --format <json|human>   output format (default: json)
  -h, --help              show help
`

type sessionFormat string

const (
	sessionFormatJSON  sessionFormat = "json"
	sessionFormatHuman sessionFormat = "human"
)

// sessionCommand implements the "archon session <id>" command.
type sessionCommand struct {
	clientFactory sessionClientFactory
	stdout        io.Writer
	stderr        io.Writer
}

func newSessionCommand(factory sessionClientFactory, stdout, stderr io.Writer) *sessionCommand {
	return &sessionCommand{clientFactory: factory, stdout: stdout, stderr: stderr}
}

func (cmd *sessionCommand) Run(args []string) error {
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help") {
		_, _ = fmt.Fprint(cmd.stderr, sessionHelpText)
		return nil
	}

	if len(args) == 0 {
		_, _ = fmt.Fprint(cmd.stderr, sessionHelpText)
		return fmt.Errorf("session id is required")
	}

	sessionID := args[0]
	format := sessionFormatJSON

	remaining := args[1:]
	for i := 0; i < len(remaining); i++ {
		switch {
		case remaining[i] == "--format" && i+1 < len(remaining):
			i++
			f := strings.ToLower(remaining[i])
			switch f {
			case "json", "human":
				format = sessionFormat(f)
			default:
				return fmt.Errorf("unknown format %q (use json or human)", remaining[i])
			}
		case strings.HasPrefix(remaining[i], "--format="):
			f := strings.ToLower(strings.TrimPrefix(remaining[i], "--format="))
			switch f {
			case "json", "human":
				format = sessionFormat(f)
			default:
				return fmt.Errorf("unknown format %q (use json or human)", f)
			}
		default:
			return fmt.Errorf("unknown flag: %s", remaining[i])
		}
	}

	client, err := cmd.clientFactory()
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	ctx := context.Background()
	if err := client.EnsureDaemon(ctx); err != nil {
		return err
	}

	session, err := client.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}

	return cmd.writeOutput(session, format)
}

func (cmd *sessionCommand) writeOutput(s *types.Session, format sessionFormat) error {
	switch format {
	case sessionFormatHuman:
		return cmd.writeHuman(s)
	default:
		return cmd.writeJSON(s)
	}
}

func (cmd *sessionCommand) writeJSON(s *types.Session) error {
	enc := json.NewEncoder(cmd.stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(s); err != nil {
		return fmt.Errorf("failed to encode session: %w", err)
	}
	return nil
}

func (cmd *sessionCommand) writeHuman(s *types.Session) error {
	w := cmd.stdout
	fmt.Fprintf(w, "ID:         %s\n", s.ID)
	fmt.Fprintf(w, "Status:     %s\n", s.Status)
	fmt.Fprintf(w, "Provider:   %s\n", s.Provider)
	if s.Title != "" {
		fmt.Fprintf(w, "Title:      %s\n", s.Title)
	}
	if s.PID != 0 {
		fmt.Fprintf(w, "PID:        %d\n", s.PID)
	}
	if s.Cwd != "" {
		fmt.Fprintf(w, "Cwd:        %s\n", s.Cwd)
	}
	if s.Cmd != "" {
		fmt.Fprintf(w, "Cmd:        %s\n", s.Cmd)
	}
	if len(s.Args) > 0 {
		fmt.Fprintf(w, "Args:       %s\n", strings.Join(s.Args, " "))
	}
	if len(s.Tags) > 0 {
		fmt.Fprintf(w, "Tags:       %s\n", strings.Join(s.Tags, ", "))
	}
	if !s.CreatedAt.IsZero() {
		fmt.Fprintf(w, "Created:    %s\n", s.CreatedAt.Format("2006-01-02 15:04:05"))
	}
	if s.StartedAt != nil {
		fmt.Fprintf(w, "Started:    %s\n", s.StartedAt.Format("2006-01-02 15:04:05"))
	}
	if s.ExitedAt != nil {
		fmt.Fprintf(w, "Exited:     %s\n", s.ExitedAt.Format("2006-01-02 15:04:05"))
	}
	if s.ExitCode != nil {
		fmt.Fprintf(w, "Exit Code:  %d\n", *s.ExitCode)
	}
	return nil
}
