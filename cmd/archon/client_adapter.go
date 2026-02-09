package main

import (
	"context"

	"control/internal/app"
	controlclient "control/internal/client"
	"control/internal/types"
)

type clientFactory func() (commandClient, error)

type commandClient interface {
	EnsureDaemon(ctx context.Context) error
	EnsureDaemonVersion(ctx context.Context, expectedVersion string, restart bool) error
	ListSessions(ctx context.Context) ([]*types.Session, error)
	StartSession(ctx context.Context, req controlclient.StartSessionRequest) (*types.Session, error)
	KillSession(ctx context.Context, id string) error
	TailItems(ctx context.Context, id string, lines int) (*controlclient.TailItemsResponse, error)
	ShutdownDaemon(ctx context.Context) error
	Health(ctx context.Context) (*controlclient.HealthResponse, error)
	RunUI() error
}

type controlClientAdapter struct {
	client *controlclient.Client
}

func newControlClient() (commandClient, error) {
	client, err := controlclient.New()
	if err != nil {
		return nil, err
	}
	return &controlClientAdapter{client: client}, nil
}

func (c *controlClientAdapter) EnsureDaemon(ctx context.Context) error {
	return c.client.EnsureDaemon(ctx)
}

func (c *controlClientAdapter) EnsureDaemonVersion(ctx context.Context, expectedVersion string, restart bool) error {
	return c.client.EnsureDaemonVersion(ctx, expectedVersion, restart)
}

func (c *controlClientAdapter) ListSessions(ctx context.Context) ([]*types.Session, error) {
	return c.client.ListSessions(ctx)
}

func (c *controlClientAdapter) StartSession(ctx context.Context, req controlclient.StartSessionRequest) (*types.Session, error) {
	return c.client.StartSession(ctx, req)
}

func (c *controlClientAdapter) KillSession(ctx context.Context, id string) error {
	return c.client.KillSession(ctx, id)
}

func (c *controlClientAdapter) TailItems(ctx context.Context, id string, lines int) (*controlclient.TailItemsResponse, error) {
	return c.client.TailItems(ctx, id, lines)
}

func (c *controlClientAdapter) ShutdownDaemon(ctx context.Context) error {
	return c.client.ShutdownDaemon(ctx)
}

func (c *controlClientAdapter) Health(ctx context.Context) (*controlclient.HealthResponse, error) {
	return c.client.Health(ctx)
}

func (c *controlClientAdapter) RunUI() error {
	return app.Run(c.client)
}
