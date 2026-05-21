package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"time"
)

type LoginCommand struct {
	stdout      io.Writer
	stderr      io.Writer
	newClient   cloudAuthClientFactory
	openBrowser browserOpener
	sleep       func(context.Context, time.Duration) error
}

func NewLoginCommand(stdout, stderr io.Writer, newClient cloudAuthClientFactory, openBrowser browserOpener) *LoginCommand {
	return &LoginCommand{
		stdout:      stdout,
		stderr:      stderr,
		newClient:   newClient,
		openBrowser: openBrowser,
		sleep:       sleepContext,
	}
}

func (c *LoginCommand) Run(args []string) error {
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	fs.SetOutput(c.stderr)
	noBrowser := fs.Bool("no-browser", false, "skip automatic browser open")
	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx := context.Background()
	client, err := c.newClient()
	if err != nil {
		return err
	}
	if err := client.EnsureDaemon(ctx); err != nil {
		return err
	}

	auth, err := client.StartCloudLogin(ctx)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(c.stdout, "Visit: %s\nCode: %s\n", auth.VerificationURI, auth.UserCode)

	if !*noBrowser && c.openBrowser != nil {
		target := auth.VerificationURIComplete
		if target == "" {
			target = auth.VerificationURI
		}
		if err := c.openBrowser(ctx, target); err != nil {
			_, _ = fmt.Fprintf(c.stderr, "browser open failed: %v\n", err)
		}
	}

	pollSeconds := auth.Interval
	if pollSeconds <= 0 {
		pollSeconds = 5
	}

	for {
		resp, err := client.PollCloudLogin(ctx)
		if err != nil {
			return err
		}
		switch resp.Status {
		case "approved":
			if resp.Auth != nil && resp.Auth.User != nil && resp.Auth.User.Email != "" {
				_, _ = fmt.Fprintf(c.stdout, "Logged in as %s\n", resp.Auth.User.Email)
			}
			return nil
		case "authorization_pending":
			if err := c.sleep(ctx, time.Duration(pollSeconds)*time.Second); err != nil {
				return err
			}
		case "slow_down":
			pollSeconds += 5
			if err := c.sleep(ctx, time.Duration(pollSeconds)*time.Second); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unexpected cloud login status: %s", resp.Status)
		}
	}
}

func sleepContext(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}
