package daemon

import (
	"path/filepath"
	"strings"

	"control/internal/types"
)

type codexFileSearchResultMapper interface {
	Map(roots []FileSearchRoot, response *codexFuzzyFileSearchResponse, limit int) []types.FileSearchCandidate
}

type defaultCodexFileSearchResultMapper struct{}

func codexFileSearchResultMapperOrDefault(mapper codexFileSearchResultMapper) codexFileSearchResultMapper {
	if mapper != nil {
		return mapper
	}
	return defaultCodexFileSearchResultMapper{}
}

func (defaultCodexFileSearchResultMapper) Map(roots []FileSearchRoot, response *codexFuzzyFileSearchResponse, limit int) []types.FileSearchCandidate {
	if response == nil || len(response.Files) == 0 {
		return nil
	}
	rootsByPath := make(map[string]FileSearchRoot, len(roots))
	for _, root := range roots {
		root.Path = filepath.Clean(strings.TrimSpace(root.Path))
		root.DisplayBase = filepath.Clean(strings.TrimSpace(root.DisplayBase))
		if root.Path == "" {
			continue
		}
		rootsByPath[root.Path] = root
	}

	merged := make(map[string]scoredFileSearchCandidate, len(response.Files))
	for _, raw := range response.Files {
		entry, ok := normalizeCodexFileSearchCandidate(raw, rootsByPath)
		if !ok {
			continue
		}
		if existing, ok := merged[entry.candidate.Path]; ok {
			if preferFileSearchCandidate(entry, existing) {
				merged[entry.candidate.Path] = entry
			}
			continue
		}
		merged[entry.candidate.Path] = entry
	}

	entries := make([]scoredFileSearchCandidate, 0, len(merged))
	for _, entry := range merged {
		entries = append(entries, entry)
	}
	sortFileSearchCandidates(entries)
	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}

	out := make([]types.FileSearchCandidate, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry.candidate)
	}
	return out
}

func normalizeCodexFileSearchCandidate(raw codexFuzzyFileSearchResult, rootsByPath map[string]FileSearchRoot) (scoredFileSearchCandidate, bool) {
	rootPath := filepath.Clean(strings.TrimSpace(raw.Root))
	root, ok := rootsByPath[rootPath]
	if !ok {
		return scoredFileSearchCandidate{}, false
	}
	candidate, ok := normalizedFileSearchCandidate(raw.Path, root)
	if !ok {
		return scoredFileSearchCandidate{}, false
	}
	candidate.Score = float64(raw.Score)
	return scoredFileSearchCandidate{
		candidate: candidate,
		primary:   root.Primary,
	}, true
}
