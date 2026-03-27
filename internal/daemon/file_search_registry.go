package daemon

import "control/internal/logging"

func newDaemonFileSearchRuntimeRegistry(stores *Stores, paths WorkspacePathResolver, logger logging.Logger) FileSearchRuntimeRegistry {
	entries := make(map[string]FileSearchProvider, 3)
	roots := NewDaemonFileSearchRootResolver(stores, paths)
	codexEnv := NewDaemonCodexFileSearchEnvironmentResolver(stores, paths)
	registerOpenCodeFileSearchProviders(entries, roots)
	registerCodexFileSearchProvider(entries, codexEnv, logger)
	return NewFileSearchProviderRegistry(entries)
}

func registerOpenCodeFileSearchProviders(entries map[string]FileSearchProvider, roots FileSearchRootResolver) {
	if entries == nil {
		return
	}
	entries["opencode"] = NewOpenCodeFileSearchProvider("opencode", roots)
	entries["kilocode"] = NewOpenCodeFileSearchProvider("kilocode", roots)
}

func registerCodexFileSearchProvider(entries map[string]FileSearchProvider, environment codexFileSearchEnvironmentProvider, logger logging.Logger) {
	if entries == nil {
		return
	}
	entries["codex"] = NewCodexFileSearchProvider("codex", environment, logger)
}
