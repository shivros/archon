package daemon

import (
	"context"
	"path/filepath"
	"strings"

	"control/internal/types"
)

type codexFileSearchEnvironment struct {
	Cwd       string
	CodexHome string
	Roots     []FileSearchRoot
}

type codexFileSearchEnvironmentProvider interface {
	Environment(ctx context.Context, scope types.FileSearchScope) (codexFileSearchEnvironment, error)
}

type codexHomeResolver interface {
	Resolve(cwd, workspacePath string) string
}

type defaultCodexHomeResolver struct{}

func (defaultCodexHomeResolver) Resolve(cwd, workspacePath string) string {
	return resolveCodexHome(cwd, workspacePath)
}

type daemonCodexFileSearchEnvironmentProvider struct {
	rootResolver  FileSearchRootResolver
	contextLoader fileSearchRootContextLoader
	homeResolver  codexHomeResolver
}

func NewDaemonCodexFileSearchEnvironmentResolver(stores *Stores, paths WorkspacePathResolver) codexFileSearchEnvironmentProvider {
	return daemonCodexFileSearchEnvironmentProvider{
		rootResolver:  NewDaemonFileSearchRootResolver(stores, paths),
		contextLoader: newDaemonFileSearchRootContextLoader(stores),
		homeResolver:  defaultCodexHomeResolver{},
	}
}

func codexFileSearchEnvironmentProviderOrDefault(provider codexFileSearchEnvironmentProvider) codexFileSearchEnvironmentProvider {
	if provider != nil {
		return provider
	}
	return daemonCodexFileSearchEnvironmentProvider{
		rootResolver:  NewPassthroughFileSearchRootResolver(),
		contextLoader: newDaemonFileSearchRootContextLoader(nil),
		homeResolver:  defaultCodexHomeResolver{},
	}
}

func (p daemonCodexFileSearchEnvironmentProvider) Environment(ctx context.Context, scope types.FileSearchScope) (codexFileSearchEnvironment, error) {
	scope = normalizeFileSearchScope(scope)
	roots, err := p.rootResolver.ResolveRoots(ctx, scope)
	if err != nil {
		return codexFileSearchEnvironment{}, err
	}
	if len(roots) == 0 {
		return codexFileSearchEnvironment{}, invalidError("file search scope must resolve to a search root", nil)
	}

	workspacePath := ""
	if p.contextLoader != nil {
		contextState, err := p.contextLoader.Load(ctx, scope)
		if err != nil {
			return codexFileSearchEnvironment{}, err
		}
		if contextState.workspace != nil {
			workspacePath = strings.TrimSpace(contextState.workspace.RepoPath)
		}
	}

	cwd := filepath.Clean(strings.TrimSpace(roots[0].Path))
	homeResolver := p.homeResolver
	if homeResolver == nil {
		homeResolver = defaultCodexHomeResolver{}
	}
	return codexFileSearchEnvironment{
		Cwd:       cwd,
		CodexHome: homeResolver.Resolve(cwd, workspacePath),
		Roots:     append([]FileSearchRoot(nil), roots...),
	}, nil
}
