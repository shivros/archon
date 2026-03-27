package daemon

import (
	"context"
	"path/filepath"
	"strings"
	"sync"

	"control/internal/logging"
)

type codexFileSearchClient interface {
	FuzzyFileSearch(ctx context.Context, query string, roots []string) (*codexFuzzyFileSearchResponse, error)
	Close()
}

type codexFileSearchClientFactory interface {
	Client(ctx context.Context, cwd, codexHome string, logger logging.Logger) (codexFileSearchClient, error)
}

type defaultCodexFileSearchClientFactory struct{}

func (defaultCodexFileSearchClientFactory) Client(ctx context.Context, cwd, codexHome string, logger logging.Logger) (codexFileSearchClient, error) {
	return startCodexAppServerWithOptions(ctx, cwd, codexHome, logger, codexInitializeOptions{
		ClientName:      "archon_file_search",
		ClientTitle:     "Archon File Search",
		ClientVersion:   "dev",
		ExperimentalAPI: true,
	})
}

type codexFileSearchClientManager interface {
	Client(ctx context.Context, env codexFileSearchEnvironment) (codexFileSearchClient, error)
	Close() error
}

type reusableCodexFileSearchClientManager struct {
	factory codexFileSearchClientFactory
	logger  logging.Logger

	mu        sync.Mutex
	client    codexFileSearchClient
	clientCwd string
	codexHome string
}

func newReusableCodexFileSearchClientManager(factory codexFileSearchClientFactory, logger logging.Logger) codexFileSearchClientManager {
	if logger == nil {
		logger = logging.Nop()
	}
	if factory == nil {
		factory = defaultCodexFileSearchClientFactory{}
	}
	return &reusableCodexFileSearchClientManager{
		factory: factory,
		logger:  logger,
	}
}

func (m *reusableCodexFileSearchClientManager) Client(ctx context.Context, env codexFileSearchEnvironment) (codexFileSearchClient, error) {
	if m == nil {
		return nil, unavailableError("file search client is not available", nil)
	}
	cwd := filepath.Clean(strings.TrimSpace(env.Cwd))
	codexHome := strings.TrimSpace(env.CodexHome)

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.client != nil && m.clientCwd == cwd && m.codexHome == codexHome {
		return m.client, nil
	}
	if m.client != nil {
		m.client.Close()
		m.client = nil
	}
	client, err := m.factory.Client(ctx, cwd, codexHome, m.logger)
	if err != nil {
		return nil, err
	}
	m.client = client
	m.clientCwd = cwd
	m.codexHome = codexHome
	return client, nil
}

func (m *reusableCodexFileSearchClientManager) Close() error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	client := m.client
	m.client = nil
	m.clientCwd = ""
	m.codexHome = ""
	m.mu.Unlock()
	if client != nil {
		client.Close()
	}
	return nil
}
