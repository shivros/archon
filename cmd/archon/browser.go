package main

import (
	"context"
	"os/exec"
)

type browserOpener func(ctx context.Context, url string) error

func openBrowserURL(ctx context.Context, url string) error {
	return exec.CommandContext(ctx, "xdg-open", url).Start()
}
