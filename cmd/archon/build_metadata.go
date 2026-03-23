package main

import (
	"runtime/debug"
	"strings"
)

// These values are set at build time via -ldflags -X.
// Defaults keep local development builds coherent.
var (
	appVersion   = "dev"
	appCommit    = "none"
	appBuildDate = "unknown"
)

type buildMetadata struct {
	Version   string
	Commit    string
	BuildDate string
}

type buildMetadataProvider interface {
	Snapshot() buildMetadata
}

type vcsBuildInfo struct {
	Revision string
	Modified bool
}

type vcsBuildInfoProvider interface {
	Read() (vcsBuildInfo, bool)
}

type readBuildInfoFunc func() (*debug.BuildInfo, bool)

type ldflagsMetadataProvider struct{}

func (ldflagsMetadataProvider) Snapshot() buildMetadata {
	return buildMetadata{
		Version:   normalizedMetadataValue(appVersion, "dev"),
		Commit:    normalizedMetadataValue(appCommit, "none"),
		BuildDate: normalizedMetadataValue(appBuildDate, "unknown"),
	}
}

type runtimeVCSBuildInfoProvider struct {
	readBuildInfo readBuildInfoFunc
}

func newRuntimeVCSBuildInfoProvider(reader readBuildInfoFunc) runtimeVCSBuildInfoProvider {
	if reader == nil {
		reader = debug.ReadBuildInfo
	}
	return runtimeVCSBuildInfoProvider{readBuildInfo: reader}
}

func (p runtimeVCSBuildInfoProvider) Read() (vcsBuildInfo, bool) {
	info, ok := p.readBuildInfo()
	if !ok {
		return vcsBuildInfo{}, false
	}
	return parseVCSBuildInfo(info.Settings), true
}

type buildMetadataResolver struct {
	baseProvider buildMetadataProvider
	vcsProvider  vcsBuildInfoProvider
}

func newBuildMetadataResolver(baseProvider buildMetadataProvider, vcsProvider vcsBuildInfoProvider) buildMetadataResolver {
	if baseProvider == nil {
		baseProvider = ldflagsMetadataProvider{}
	}
	if vcsProvider == nil {
		vcsProvider = newRuntimeVCSBuildInfoProvider(nil)
	}
	return buildMetadataResolver{
		baseProvider: baseProvider,
		vcsProvider:  vcsProvider,
	}
}

func defaultBuildMetadataProvider() buildMetadataProvider {
	return newBuildMetadataResolver(ldflagsMetadataProvider{}, newRuntimeVCSBuildInfoProvider(nil))
}

func buildVersion() string {
	return defaultBuildMetadataProvider().Snapshot().Version
}

func (r buildMetadataResolver) Snapshot() buildMetadata {
	metadata := r.baseProvider.Snapshot()
	useVCSVersion := metadata.Version == "dev"
	useVCSCommit := metadata.Commit == "none"

	if info, ok := r.vcsProvider.Read(); ok {
		if useVCSCommit && info.Revision != "" {
			metadata.Commit = info.Revision
		}
		if useVCSVersion && info.Revision != "" {
			metadata.Version = info.Revision
		}
		if info.Modified {
			if useVCSCommit && metadata.Commit != "none" && !strings.HasSuffix(metadata.Commit, "-dirty") {
				metadata.Commit += "-dirty"
			}
			if useVCSVersion && metadata.Version != "dev" && !strings.HasSuffix(metadata.Version, "-dirty") {
				metadata.Version += "-dirty"
			}
		}
	}

	return metadata
}

func normalizedMetadataValue(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func parseVCSBuildInfo(settings []debug.BuildSetting) vcsBuildInfo {
	result := vcsBuildInfo{}
	for _, setting := range settings {
		switch setting.Key {
		case "vcs.revision":
			result.Revision = strings.TrimSpace(setting.Value)
		case "vcs.modified":
			result.Modified = strings.EqualFold(strings.TrimSpace(setting.Value), "true")
		}
	}
	return result
}
