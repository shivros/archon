package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/atotto/clipboard"
	osc52 "github.com/aymanbagabas/go-osc52/v2"
)

type clipboardMethod uint8

const (
	clipboardMethodSystem clipboardMethod = iota
	clipboardMethodOSC52
)

var clipboardWriteAll = clipboard.WriteAll
var clipboardWriteOSC52 = writeOSC52Clipboard

func copyTextToClipboard(text string) (clipboardMethod, error) {
	if err := clipboardWriteAll(text); err == nil {
		return clipboardMethodSystem, nil
	} else {
		if oscErr := clipboardWriteOSC52(text); oscErr == nil {
			return clipboardMethodOSC52, nil
		} else {
			return clipboardMethodSystem, combineClipboardErrors(err, oscErr)
		}
	}
}

func (m *Model) copyWithStatus(text, success string) bool {
	_, err := copyTextToClipboard(text)
	if err != nil {
		m.setCopyStatusError("copy failed: " + err.Error())
		return false
	}
	m.setCopyStatusInfo(success)
	return true
}

func writeOSC52Clipboard(text string) error {
	if !shouldAttemptOSC52() {
		return errors.New("OSC52 unavailable for this terminal")
	}
	tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("open /dev/tty: %w", err)
	}
	defer tty.Close()
	return writeOSC52Sequence(tty, text)
}

func writeOSC52Sequence(w io.Writer, text string) error {
	termName := strings.ToLower(strings.TrimSpace(os.Getenv("TERM")))
	if os.Getenv("TMUX") != "" {
		// Emit both plain and tmux-wrapped OSC52 for compatibility with
		// different tmux clipboard configurations.
		if _, err := osc52.New(text).WriteTo(w); err != nil {
			return err
		}
		if _, err := osc52.New(text).Tmux().WriteTo(w); err != nil {
			return err
		}
		return nil
	} else if strings.HasPrefix(termName, "screen") {
		if _, err := osc52.New(text).Screen().WriteTo(w); err != nil {
			return err
		}
		return nil
	}
	if _, err := osc52.New(text).WriteTo(w); err != nil {
		return err
	}
	return nil
}

func shouldAttemptOSC52() bool {
	disabled := strings.ToLower(strings.TrimSpace(os.Getenv("ARCHON_DISABLE_OSC52")))
	switch disabled {
	case "1", "true", "yes", "on":
		return false
	}
	termName := strings.TrimSpace(os.Getenv("TERM"))
	if termName == "" || strings.EqualFold(termName, "dumb") {
		return false
	}
	return true
}

func combineClipboardErrors(systemErr, oscErr error) error {
	systemMsg := humanizeClipboardError(systemErr)
	oscMsg := humanizeClipboardError(oscErr)
	if missingDisplay() {
		return fmt.Errorf("no GUI clipboard available (DISPLAY/WAYLAND_DISPLAY unset); OSC52 fallback failed: %s", oscMsg)
	}
	return fmt.Errorf("system clipboard failed: %s; OSC52 fallback failed: %s", systemMsg, oscMsg)
}

func humanizeClipboardError(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.TrimSpace(err.Error())
	if msg == "exit status 1" {
		if missingDisplay() {
			return "no GUI clipboard available (DISPLAY/WAYLAND_DISPLAY unset)"
		}
		return "clipboard helper exited with status 1"
	}
	return msg
}

func missingDisplay() bool {
	return strings.TrimSpace(os.Getenv("DISPLAY")) == "" && strings.TrimSpace(os.Getenv("WAYLAND_DISPLAY")) == ""
}
