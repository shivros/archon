package main

import (
	"context"

	"control/internal/app"
	controlclient "control/internal/client"
	"control/internal/types"
)

type cloudAuthCommandClient interface {
	EnsureDaemon(ctx context.Context) error
	StartCloudLogin(ctx context.Context) (*controlclient.CloudDeviceAuthorizationResponse, error)
	PollCloudLogin(ctx context.Context) (*controlclient.CloudAuthPollResponse, error)
	CloudAuthStatus(ctx context.Context) (*controlclient.CloudAuthStatusResponse, error)
	LogoutCloud(ctx context.Context) (*controlclient.CloudLogoutResponse, error)
}

type sessionCommandClient interface {
	EnsureDaemon(ctx context.Context) error
	ListSessions(ctx context.Context) ([]*types.Session, error)
	StartSession(ctx context.Context, req controlclient.StartSessionRequest) (*types.Session, error)
	KillSession(ctx context.Context, id string) error
	TailItems(ctx context.Context, id string, lines int) (*controlclient.TailItemsResponse, error)
}

type daemonVersionClient interface {
	EnsureDaemon(ctx context.Context) error
	EnsureDaemonVersion(ctx context.Context, expectedVersion string, restart bool) error
	RunUI() error
}

type daemonAdminClient interface {
	ShutdownDaemon(ctx context.Context) error
	Health(ctx context.Context) (*controlclient.HealthResponse, error)
}

type cloudAuthClientFactory func() (cloudAuthCommandClient, error)
type sessionClientFactory func() (sessionCommandClient, error)
type daemonVersionClientFactory func() (daemonVersionClient, error)
type daemonAdminClientFactory func() (daemonAdminClient, error)

type controlClientAdapter struct {
	client *controlclient.Client
}

func newControlClientAdapter() (*controlClientAdapter, error) {
	client, err := controlclient.New()
	if err != nil {
		return nil, err
	}
	return &controlClientAdapter{client: client}, nil
}

func newCloudAuthClient() (cloudAuthCommandClient, error) {
	return newControlClientAdapter()
}

func newSessionClient() (sessionCommandClient, error) {
	return newControlClientAdapter()
}

func newDaemonVersionClient() (daemonVersionClient, error) {
	return newControlClientAdapter()
}

func newDaemonAdminClient() (daemonAdminClient, error) {
	return newControlClientAdapter()
}

func (c *controlClientAdapter) EnsureDaemon(ctx context.Context) error {
	return c.client.EnsureDaemon(ctx)
}

func (c *controlClientAdapter) EnsureDaemonVersion(ctx context.Context, expectedVersion string, restart bool) error {
	return c.client.EnsureDaemonVersion(ctx, expectedVersion, restart)
}

func (c *controlClientAdapter) StartCloudLogin(ctx context.Context) (*controlclient.CloudDeviceAuthorizationResponse, error) {
	return c.client.StartCloudLogin(ctx)
}

func (c *controlClientAdapter) PollCloudLogin(ctx context.Context) (*controlclient.CloudAuthPollResponse, error) {
	return c.client.PollCloudLogin(ctx)
}

func (c *controlClientAdapter) CloudAuthStatus(ctx context.Context) (*controlclient.CloudAuthStatusResponse, error) {
	return c.client.CloudAuthStatus(ctx)
}

func (c *controlClientAdapter) LogoutCloud(ctx context.Context) (*controlclient.CloudLogoutResponse, error) {
	return c.client.LogoutCloud(ctx)
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
