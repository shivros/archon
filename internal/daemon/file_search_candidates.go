package daemon

import (
	"path/filepath"
	"sort"
	"strings"

	"control/internal/types"
)

type FileSearchCandidateNormalizer interface {
	Normalize(query string, roots []FileSearchRoot, rawResults map[string][]string, limit int) []types.FileSearchCandidate
}

type defaultFileSearchCandidateNormalizer struct{}

func fileSearchCandidateNormalizerOrDefault(normalizer FileSearchCandidateNormalizer) FileSearchCandidateNormalizer {
	if normalizer != nil {
		return normalizer
	}
	return defaultFileSearchCandidateNormalizer{}
}

func (defaultFileSearchCandidateNormalizer) Normalize(query string, roots []FileSearchRoot, rawResults map[string][]string, limit int) []types.FileSearchCandidate {
	merged := make(map[string]scoredFileSearchCandidate)
	rootsByPath := make(map[string]FileSearchRoot, len(roots))
	for _, root := range roots {
		rootsByPath[root.Path] = root
	}
	for rootPath, paths := range rawResults {
		root, ok := rootsByPath[rootPath]
		if !ok {
			continue
		}
		for _, rawPath := range paths {
			entry, ok := normalizeFileSearchCandidate(rawPath, root, query)
			if !ok {
				continue
			}
			if existing, exists := merged[entry.candidate.Path]; exists {
				if preferFileSearchCandidate(entry, existing) {
					merged[entry.candidate.Path] = entry
				}
				continue
			}
			merged[entry.candidate.Path] = entry
		}
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

type scoredFileSearchCandidate struct {
	candidate types.FileSearchCandidate
	primary   bool
}

func normalizeFileSearchCandidate(rawPath string, root FileSearchRoot, query string) (scoredFileSearchCandidate, bool) {
	candidate, ok := normalizedFileSearchCandidate(rawPath, root)
	if !ok {
		return scoredFileSearchCandidate{}, false
	}
	candidate.Score = scoreFileSearchCandidate(query, candidate.DisplayPath, candidate.Path)
	return scoredFileSearchCandidate{
		candidate: candidate,
		primary:   root.Primary,
	}, true
}

func normalizedFileSearchCandidate(rawPath string, root FileSearchRoot) (types.FileSearchCandidate, bool) {
	rawPath = strings.TrimSpace(rawPath)
	root.Path = filepath.Clean(strings.TrimSpace(root.Path))
	root.DisplayBase = filepath.Clean(strings.TrimSpace(root.DisplayBase))
	if rawPath == "" || root.Path == "" {
		return types.FileSearchCandidate{}, false
	}

	absolutePath := rawPath
	if !filepath.IsAbs(absolutePath) {
		absolutePath = filepath.Join(root.Path, rawPath)
	}
	absolutePath = filepath.Clean(absolutePath)

	displayPath := absolutePath
	if root.DisplayBase != "" {
		if rel, err := filepath.Rel(root.DisplayBase, absolutePath); err == nil && strings.TrimSpace(rel) != "" {
			displayPath = filepath.Clean(rel)
		}
	} else if rel, err := filepath.Rel(root.Path, absolutePath); err == nil && strings.TrimSpace(rel) != "" {
		displayPath = filepath.Clean(rel)
	}

	directory := filepath.Dir(displayPath)
	if directory == "." {
		directory = ""
	}
	return types.FileSearchCandidate{
		Path:        absolutePath,
		DisplayPath: displayPath,
		Directory:   directory,
		Kind:        "file",
	}, true
}

func scoreFileSearchCandidate(query, displayPath, absolutePath string) float64 {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return 0
	}
	displayPath = strings.ToLower(strings.TrimSpace(displayPath))
	base := strings.ToLower(filepath.Base(displayPath))
	baseNoExt := strings.TrimSuffix(base, filepath.Ext(base))
	score := 10.0

	switch {
	case base == q:
		score = 100
	case baseNoExt == q:
		score = 95
	case strings.HasPrefix(base, q):
		score = 90
	case strings.Contains(base, q):
		score = 80
	case strings.HasPrefix(displayPath, q):
		score = 70
	case strings.Contains(displayPath, q):
		score = 60
	case strings.Contains(strings.ToLower(strings.TrimSpace(absolutePath)), q):
		score = 50
	}

	score -= float64(len(displayPath)) / 1000
	return score
}

func preferFileSearchCandidate(left, right scoredFileSearchCandidate) bool {
	if left.candidate.Score != right.candidate.Score {
		return left.candidate.Score > right.candidate.Score
	}
	if left.primary != right.primary {
		return left.primary
	}
	if left.candidate.DisplayPath != right.candidate.DisplayPath {
		return left.candidate.DisplayPath < right.candidate.DisplayPath
	}
	return left.candidate.Path < right.candidate.Path
}

func sortFileSearchCandidates(entries []scoredFileSearchCandidate) {
	sort.SliceStable(entries, func(i, j int) bool {
		left := entries[i]
		right := entries[j]
		if left.candidate.Score != right.candidate.Score {
			return left.candidate.Score > right.candidate.Score
		}
		if left.primary != right.primary {
			return left.primary
		}
		if left.candidate.DisplayPath != right.candidate.DisplayPath {
			return left.candidate.DisplayPath < right.candidate.DisplayPath
		}
		return left.candidate.Path < right.candidate.Path
	})
}
