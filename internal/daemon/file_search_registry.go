package daemon

func newDaemonFileSearchRuntimeRegistry(stores *Stores, paths WorkspacePathResolver) FileSearchRuntimeRegistry {
	roots := NewDaemonFileSearchRootResolver(stores, paths)
	return NewFileSearchProviderRegistry(map[string]FileSearchProvider{
		"opencode": NewOpenCodeFileSearchProvider("opencode", roots),
		"kilocode": NewOpenCodeFileSearchProvider("kilocode", roots),
	})
}
