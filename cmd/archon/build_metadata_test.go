package main

import (
	"runtime/debug"
	"testing"
)

func TestNormalizedMetadataValueUsesFallbackForBlank(t *testing.T) {
	if got := normalizedMetadataValue("   ", "fallback"); got != "fallback" {
		t.Fatalf("expected fallback, got %q", got)
	}
	if got := normalizedMetadataValue("value", "fallback"); got != "value" {
		t.Fatalf("expected value, got %q", got)
	}
}

func TestBuildMetadataResolverFallsBackToVCSRevision(t *testing.T) {
	resolver := newBuildMetadataResolver(
		staticBuildMetadataProvider{
			metadata: buildMetadata{
				Version:   "dev",
				Commit:    "none",
				BuildDate: "unknown",
			},
		},
		staticVCSBuildInfoProvider{
			info: vcsBuildInfo{Revision: "abc123", Modified: false},
			ok:   true,
		},
	)

	metadata := resolver.Snapshot()
	if metadata.Version != "abc123" {
		t.Fatalf("expected vcs version fallback, got %q", metadata.Version)
	}
	if metadata.Commit != "abc123" {
		t.Fatalf("expected vcs commit fallback, got %q", metadata.Commit)
	}
	if metadata.BuildDate != "unknown" {
		t.Fatalf("expected original build date, got %q", metadata.BuildDate)
	}
}

func TestBuildMetadataResolverAddsDirtySuffixForVCSFallback(t *testing.T) {
	resolver := newBuildMetadataResolver(
		staticBuildMetadataProvider{
			metadata: buildMetadata{
				Version: "dev",
				Commit:  "none",
			},
		},
		staticVCSBuildInfoProvider{
			info: vcsBuildInfo{Revision: "abc123", Modified: true},
			ok:   true,
		},
	)

	metadata := resolver.Snapshot()
	if metadata.Version != "abc123-dirty" {
		t.Fatalf("expected dirty version suffix, got %q", metadata.Version)
	}
	if metadata.Commit != "abc123-dirty" {
		t.Fatalf("expected dirty commit suffix, got %q", metadata.Commit)
	}
}

func TestBuildMetadataResolverPreservesInjectedValues(t *testing.T) {
	resolver := newBuildMetadataResolver(
		staticBuildMetadataProvider{
			metadata: buildMetadata{
				Version:   "v1.2.3",
				Commit:    "deadbeef",
				BuildDate: "2026-03-23T00:00:00Z",
			},
		},
		staticVCSBuildInfoProvider{
			info: vcsBuildInfo{Revision: "abc123", Modified: true},
			ok:   true,
		},
	)

	metadata := resolver.Snapshot()
	if metadata.Version != "v1.2.3" {
		t.Fatalf("expected injected version to win, got %q", metadata.Version)
	}
	if metadata.Commit != "deadbeef" {
		t.Fatalf("expected injected commit to win, got %q", metadata.Commit)
	}
	if metadata.BuildDate != "2026-03-23T00:00:00Z" {
		t.Fatalf("expected injected build date to win, got %q", metadata.BuildDate)
	}
}

func TestNewBuildMetadataResolverDefaultsDependencies(t *testing.T) {
	resolver := newBuildMetadataResolver(nil, nil)
	metadata := resolver.Snapshot()
	if metadata.Version == "" {
		t.Fatalf("expected non-empty version")
	}
	if metadata.Commit == "" {
		t.Fatalf("expected non-empty commit")
	}
	if metadata.BuildDate == "" {
		t.Fatalf("expected non-empty build date")
	}
}

func TestRuntimeVCSBuildInfoProviderReadReturnsFalseWhenBuildInfoUnavailable(t *testing.T) {
	provider := newRuntimeVCSBuildInfoProvider(func() (*debug.BuildInfo, bool) {
		return nil, false
	})

	info, ok := provider.Read()
	if ok {
		t.Fatalf("expected read to report unavailable build info")
	}
	if info != (vcsBuildInfo{}) {
		t.Fatalf("expected zero value info, got %#v", info)
	}
}

func TestRuntimeVCSBuildInfoProviderReadParsesBuildInfoSettings(t *testing.T) {
	provider := newRuntimeVCSBuildInfoProvider(func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{
			Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "  deadbeef  "},
				{Key: "vcs.modified", Value: " TrUe "},
			},
		}, true
	})

	info, ok := provider.Read()
	if !ok {
		t.Fatalf("expected read to succeed")
	}
	if info.Revision != "deadbeef" {
		t.Fatalf("expected trimmed revision, got %q", info.Revision)
	}
	if !info.Modified {
		t.Fatalf("expected modified=true, got false")
	}
}

func TestParseVCSBuildInfoIgnoresUnknownSettings(t *testing.T) {
	info := parseVCSBuildInfo([]debug.BuildSetting{
		{Key: "vcs.revision", Value: "abc123"},
		{Key: "vcs.modified", Value: "false"},
		{Key: "custom.setting", Value: "ignored"},
	})
	if info.Revision != "abc123" {
		t.Fatalf("expected revision abc123, got %q", info.Revision)
	}
	if info.Modified {
		t.Fatalf("expected modified=false")
	}
}

type staticVCSBuildInfoProvider struct {
	info vcsBuildInfo
	ok   bool
}

func (p staticVCSBuildInfoProvider) Read() (vcsBuildInfo, bool) {
	return p.info, p.ok
}
