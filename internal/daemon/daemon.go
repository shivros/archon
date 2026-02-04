package daemon

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"control/internal/types"
)

type Daemon struct {
	addr    string
	token   string
	version string
	server  *http.Server
	manager *SessionManager
	stores  *Stores
}

type Stores struct {
	Workspaces  WorkspaceStore
	Worktrees   WorktreeStore
	AppState    AppStateStore
	Keymap      KeymapStore
	SessionMeta SessionMetaStore
	Sessions    SessionIndexStore
}

type WorkspaceStore interface {
	List(ctx context.Context) ([]*types.Workspace, error)
	Get(ctx context.Context, id string) (*types.Workspace, bool, error)
	Add(ctx context.Context, workspace *types.Workspace) (*types.Workspace, error)
	Update(ctx context.Context, workspace *types.Workspace) (*types.Workspace, error)
	Delete(ctx context.Context, id string) error
}

type WorktreeStore interface {
	ListWorktrees(ctx context.Context, workspaceID string) ([]*types.Worktree, error)
	AddWorktree(ctx context.Context, workspaceID string, worktree *types.Worktree) (*types.Worktree, error)
	DeleteWorktree(ctx context.Context, workspaceID, worktreeID string) error
}

type AppStateStore interface {
	Load(ctx context.Context) (*types.AppState, error)
	Save(ctx context.Context, state *types.AppState) error
}

type KeymapStore interface {
	Load(ctx context.Context) (*types.Keymap, error)
	Save(ctx context.Context, keymap *types.Keymap) error
}

type ProviderRegistry interface {
	List() []types.ProviderInfo
}

type SessionMetaStore interface {
	List(ctx context.Context) ([]*types.SessionMeta, error)
	Get(ctx context.Context, sessionID string) (*types.SessionMeta, bool, error)
	Upsert(ctx context.Context, meta *types.SessionMeta) (*types.SessionMeta, error)
	Delete(ctx context.Context, sessionID string) error
}

type SessionIndexStore interface {
	ListRecords(ctx context.Context) ([]*types.SessionRecord, error)
	GetRecord(ctx context.Context, sessionID string) (*types.SessionRecord, bool, error)
	UpsertRecord(ctx context.Context, record *types.SessionRecord) (*types.SessionRecord, error)
	DeleteRecord(ctx context.Context, sessionID string) error
}

func New(addr, token, version string, manager *SessionManager, stores *Stores) *Daemon {
	if manager != nil && stores != nil && stores.SessionMeta != nil {
		manager.SetMetaStore(stores.SessionMeta)
	}
	if manager != nil && stores != nil && stores.Sessions != nil {
		manager.SetSessionStore(stores.Sessions)
	}
	return &Daemon{
		addr:    addr,
		token:   token,
		version: version,
		manager: manager,
		stores:  stores,
	}
}

func (d *Daemon) Run(ctx context.Context) error {
	api := &API{
		Version: d.version,
		Manager: d.manager,
		Stores:  d.stores,
	}
	syncer := NewCodexSyncer(d.stores)
	api.Syncer = syncer
	api.LiveCodex = NewCodexLiveManager()

	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	handler := TokenAuthMiddleware(d.token, mux)
	d.server = &http.Server{
		Addr:    d.addr,
		Handler: handler,
	}
	api.Shutdown = d.server.Shutdown

	go func() {
		_ = syncer.SyncAll(context.Background())
	}()

	errCh := make(chan error, 1)
	go func() {
		log.Printf("daemon listening on http://%s", d.addr)
		errCh <- d.server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := d.server.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
