package app

import (
	"context"
	"fmt"
	"strings"
)

type defaultFileLinkOpener struct {
	policy FileLinkOpenPolicy
	runner FileLinkCommandRunner
}

func newDefaultFileLinkOpener() FileLinkOpener {
	return defaultFileLinkOpener{
		policy: newDefaultFileLinkOpenPolicy(),
		runner: defaultFileLinkCommandRunner{},
	}
}

func (o defaultFileLinkOpener) Open(ctx context.Context, target ResolvedFileLink) error {
	openTarget := strings.TrimSpace(target.OpenTarget())
	if openTarget == "" {
		return errFileLinkEmptyTarget
	}
	policy := o.policy
	if policy == nil {
		policy = newDefaultFileLinkOpenPolicy()
	}
	runner := o.runner
	if runner == nil {
		runner = defaultFileLinkCommandRunner{}
	}
	command, err := policy.BuildCommand(target)
	if err != nil {
		return err
	}
	if err := runner.Run(ctx, command); err != nil {
		return fmt.Errorf("open %q: %w", openTarget, err)
	}
	return nil
}
