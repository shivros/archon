package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os/signal"
	"strings"
	"syscall"
)

// flusher is checked at runtime to flush stdout after each NDJSON line.
// *os.File implements this on most platforms.
type flusher interface {
	Flush() error
}

type TailCommand struct {
	stdout    io.Writer
	stderr    io.Writer
	newClient sessionClientFactory
}

func NewTailCommand(stdout, stderr io.Writer, newClient sessionClientFactory) *TailCommand {
	return &TailCommand{
		stdout:    stdout,
		stderr:    stderr,
		newClient: newClient,
	}
}

func (c *TailCommand) Run(args []string) error {
	// Reorder: move all flags (with their values) before positional args
	// so Go's flag parser sees them. This allows both `tail --follow ID`
	// and `tail ID --follow`.
	reordered := reorderFlagsBeforePositional(args)

	fs := flag.NewFlagSet("tail", flag.ContinueOnError)
	fs.SetOutput(c.stderr)
	lines := fs.Int("lines", 200, "number of lines to fetch (snapshot backfill)")
	follow := fs.Bool("follow", false, "stream live events after initial snapshot (NDJSON output)")
	fs.BoolVar(follow, "f", false, "shorthand for --follow")
	stream := fs.String("stream", "combined", "stream to follow: stdout, stderr, or combined (only with --follow)")
	if err := fs.Parse(reordered); err != nil {
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

	// Snapshot-only path (no --follow).
	if !*follow {
		resp, err := client.TailItems(ctx, id, *lines)
		if err != nil {
			return err
		}
		items := resp.Items
		if items == nil {
			items = []map[string]any{}
		}
		return json.NewEncoder(c.stdout).Encode(items)
	}

	// Follow path: optional backfill, then live stream.
	return c.runFollow(ctx, client, id, *lines, *stream)
}

func (c *TailCommand) runFollow(ctx context.Context, client sessionCommandClient, id string, lines int, streamName string) error {
	// Install signal handler for clean cancellation.
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var lastBackfillItem map[string]any

	// Backfill phase: emit snapshot items as NDJSON if --lines > 0.
	if lines > 0 {
		resp, err := client.TailItems(ctx, id, lines)
		if err != nil {
			// Backfill failure is non-fatal — continue to live stream.
			// The caller will still get live events from this point forward.
		} else {
			for _, item := range resp.Items {
				line, err := json.Marshal(item)
				if err != nil {
					continue
				}
				lastBackfillItem = item
				if err := c.writeNDJSONLine(line); err != nil {
					return err
				}
			}
		}
	}

	// Live stream phase.
	ch, cancelStream, err := client.StreamTail(ctx, id, streamName)
	if err != nil {
		return err
	}
	defer cancelStream()

	for event := range ch {
		// Dedup: skip first stream event if it matches last backfill item semantically.
		if lastBackfillItem != nil {
			// Normalize the stream event to a map for comparison.
			eventBytes, err := json.Marshal(event)
			if err != nil {
				continue
			}
			var eventMap map[string]any
			if err := json.Unmarshal(eventBytes, &eventMap); err != nil {
				lastBackfillItem = nil
				if err := c.writeNDJSONLine(eventBytes); err != nil {
					return err
				}
				continue
			}
			if jsonMapsEqual(eventMap, lastBackfillItem) {
				lastBackfillItem = nil // only skip one duplicate
				continue
			}
			lastBackfillItem = nil
			if err := c.writeNDJSONLine(eventBytes); err != nil {
				return err
			}
			continue
		}

		line, err := json.Marshal(event)
		if err != nil {
			continue
		}
		if err := c.writeNDJSONLine(line); err != nil {
			return err
		}
	}

	// Channel closed.
	// If context was cancelled (signal), this is a clean exit.
	select {
	case <-ctx.Done():
		return nil
	default:
	}
	// Otherwise the daemon closed the stream — also a clean exit.
	return nil
}

// jsonMapsEqual compares two maps[string]any for semantic JSON equality.
func jsonMapsEqual(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		bv, ok := b[k]
		if !ok {
			return false
		}
		// Normalize both sides through JSON round-trip for nested values.
		aj, err := json.Marshal(av)
		if err != nil {
			return false
		}
		bj, err := json.Marshal(bv)
		if err != nil {
			return false
		}
		if string(aj) != string(bj) {
			return false
		}
	}
	return true
}

// reorderFlagsBeforePositional reorders args so all flags come before
// positional arguments, preserving flag-value pairs.
// It uses the known flag set for the tail command to distinguish bool flags
// from value-taking flags, so it correctly handles `--follow ID --lines 5`.
func reorderFlagsBeforePositional(args []string) []string {
	// Known bool flags for the tail command.
	boolFlags := map[string]bool{
		"--follow": true,
		"-follow":  true,
		"-f":       true,
	}

	var flags []string
	var positional []string
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], "-") {
			flags = append(flags, args[i])
			// If this is not a bool flag and the next arg doesn't start with "-",
			// it's the value for this flag.
			if !boolFlags[args[i]] && !strings.Contains(args[i], "=") && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				flags = append(flags, args[i])
			}
		} else {
			positional = append(positional, args[i])
		}
	}
	return append(flags, positional...)
}

// writeNDJSONLine writes a single JSON line followed by \n and flushes if possible.
func (c *TailCommand) writeNDJSONLine(line []byte) error {
	if _, err := c.stdout.Write(line); err != nil {
		return err
	}
	if _, err := c.stdout.Write([]byte{'\n'}); err != nil {
		return err
	}
	if f, ok := c.stdout.(flusher); ok {
		_ = f.Flush()
	}
	return nil
}
