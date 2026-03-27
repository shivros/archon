package daemon

import (
	"context"
	"strings"

	"control/internal/types"
)

type FileSearchExecutor interface {
	Search(ctx context.Context, query string, roots []FileSearchRoot, limit int) ([]types.FileSearchCandidate, error)
}

type openCodeFileSearchExecutor struct {
	searcher   openCodeFileSearcher
	normalizer FileSearchCandidateNormalizer
}

func newOpenCodeFileSearchExecutor(searcher openCodeFileSearcher, normalizer FileSearchCandidateNormalizer) FileSearchExecutor {
	return openCodeFileSearchExecutor{
		searcher:   searcher,
		normalizer: fileSearchCandidateNormalizerOrDefault(normalizer),
	}
}

func (e openCodeFileSearchExecutor) Search(ctx context.Context, query string, roots []FileSearchRoot, limit int) ([]types.FileSearchCandidate, error) {
	query = strings.TrimSpace(query)
	if query == "" || len(roots) == 0 {
		return nil, nil
	}
	if e.searcher == nil {
		return nil, unavailableError("file search client is not available", nil)
	}
	rawResults := make(map[string][]string, len(roots))
	for _, root := range roots {
		paths, err := e.searcher.SearchFiles(ctx, openCodeFileSearchRequest{
			Query:     query,
			Directory: root.Path,
			Limit:     limit,
		})
		if err != nil {
			return nil, err
		}
		rawResults[root.Path] = append([]string(nil), paths...)
	}
	return e.normalizer.Normalize(query, roots, rawResults, limit), nil
}
