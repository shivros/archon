package app

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

type defaultFileLinkCommandRunner struct{}

func (defaultFileLinkCommandRunner) Run(ctx context.Context, command FileLinkOpenCommand) error {
	name := strings.TrimSpace(command.Name)
	if name == "" {
		return errors.New("open command is empty")
	}
	cmd := exec.CommandContext(ctx, name, command.Args...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("command %q failed: %w", name, err)
	}
	return nil
}
