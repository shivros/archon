package daemon

import (
	"context"
	"errors"
	"strings"
)

type openCodeFileSearcher interface {
	SearchFiles(ctx context.Context, req openCodeFileSearchRequest) ([]string, error)
}

type openCodeFileSearcherFactory interface {
	Searcher(provider string) (openCodeFileSearcher, error)
}

type defaultOpenCodeFileSearcherFactory struct{}

func (defaultOpenCodeFileSearcherFactory) Searcher(provider string) (openCodeFileSearcher, error) {
	client, err := newOpenCodeClient(resolveOpenCodeClientConfig(provider, loadCoreConfigOrDefault()))
	if err != nil {
		return nil, err
	}
	return &recoveringOpenCodeFileSearcher{
		provider: strings.TrimSpace(provider),
		client:   client,
	}, nil
}

func openCodeFileSearcherFactoryOrDefault(factory openCodeFileSearcherFactory) openCodeFileSearcherFactory {
	if factory != nil {
		return factory
	}
	return defaultOpenCodeFileSearcherFactory{}
}

type recoveringOpenCodeFileSearcher struct {
	provider string
	client   *openCodeClient
}

func (s *recoveringOpenCodeFileSearcher) SearchFiles(ctx context.Context, req openCodeFileSearchRequest) ([]string, error) {
	req = normalizeOpenCodeFileSearchRequest(req)
	if s == nil || s.client == nil {
		return nil, unavailableError("file search client is not available", nil)
	}
	results, err := s.client.SearchFiles(ctx, req)
	if err == nil {
		return results, nil
	}
	if !isOpenCodeUnreachable(err) {
		return nil, err
	}
	startedBaseURL, startErr := maybeAutoStartOpenCodeServer(s.provider, s.client.baseURL, s.client.token, nil)
	if startErr != nil {
		return nil, errors.New(err.Error() + " (auto-start failed: " + startErr.Error() + ")")
	}
	switchedClient, switchErr := cloneOpenCodeClientWithBaseURL(s.client, startedBaseURL)
	if switchErr != nil {
		return nil, switchErr
	}
	s.client = switchedClient
	return s.client.SearchFiles(ctx, req)
}
