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
	GetSession(ctx context.Context, sessionID string) (*types.Session, error)
	StartSession(ctx context.Context, req controlclient.StartSessionRequest) (*types.Session, error)
	KillSession(ctx context.Context, id string) error
	InterruptSession(ctx context.Context, id string) error
	TailItems(ctx context.Context, id string, lines int) (*controlclient.TailItemsResponse, error)
	StreamTail(ctx context.Context, id, stream string) (<-chan types.LogEvent, func(), error)
	SendMessage(ctx context.Context, sessionID string, req controlclient.SendSessionRequest) (*controlclient.SendSessionResponse, error)
	SteerSession(ctx context.Context, sessionID string, req controlclient.SteerSessionRequest) (*controlclient.SteerSessionResponse, error)
	ListApprovals(ctx context.Context, sessionID string) ([]*types.Approval, error)
	ApproveSession(ctx context.Context, sessionID string, req controlclient.ApproveSessionRequest) error
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

func (c *controlClientAdapter) GetSession(ctx context.Context, sessionID string) (*types.Session, error) {
	return c.client.GetSession(ctx, sessionID)
}

func (c *controlClientAdapter) StartSession(ctx context.Context, req controlclient.StartSessionRequest) (*types.Session, error) {
	return c.client.StartSession(ctx, req)
}

func (c *controlClientAdapter) KillSession(ctx context.Context, id string) error {
	return c.client.KillSession(ctx, id)
}

func (c *controlClientAdapter) InterruptSession(ctx context.Context, id string) error {
	return c.client.InterruptSession(ctx, id)
}

func (c *controlClientAdapter) TailItems(ctx context.Context, id string, lines int) (*controlclient.TailItemsResponse, error) {
	return c.client.TailItems(ctx, id, lines)
}

func (c *controlClientAdapter) StreamTail(ctx context.Context, id, stream string) (<-chan types.LogEvent, func(), error) {
	return c.client.TailStream(ctx, id, stream)
}

func (c *controlClientAdapter) SendMessage(ctx context.Context, sessionID string, req controlclient.SendSessionRequest) (*controlclient.SendSessionResponse, error) {
	return c.client.SendMessage(ctx, sessionID, req)
}

func (c *controlClientAdapter) SteerSession(ctx context.Context, sessionID string, req controlclient.SteerSessionRequest) (*controlclient.SteerSessionResponse, error) {
	return c.client.SteerSession(ctx, sessionID, req)
}

func (c *controlClientAdapter) ListApprovals(ctx context.Context, sessionID string) ([]*types.Approval, error) {
	return c.client.ListApprovals(ctx, sessionID)
}

func (c *controlClientAdapter) ApproveSession(ctx context.Context, sessionID string, req controlclient.ApproveSessionRequest) error {
	return c.client.ApproveSession(ctx, sessionID, req)
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
