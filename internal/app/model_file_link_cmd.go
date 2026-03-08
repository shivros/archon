package app

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
)

func WithFileLinkResolver(resolver FileLinkResolver) ModelOption {
	return func(m *Model) {
		if m == nil || resolver == nil {
			return
		}
		m.fileLinkResolver = resolver
	}
}

func WithFileLinkOpener(opener FileLinkOpener) ModelOption {
	return func(m *Model) {
		if m == nil || opener == nil {
			return
		}
		m.fileLinkOpener = opener
	}
}

func (m *Model) openFileLinkCmd(rawTarget string) tea.Cmd {
	resolver := m.fileLinkResolver
	if resolver == nil {
		resolver = defaultFileLinkResolver{}
	}
	resolved, err := resolver.Resolve(rawTarget)
	if err != nil {
		m.setStatusWarning("unsupported link: " + strings.TrimSpace(rawTarget))
		return nil
	}
	opener := m.fileLinkOpener
	if opener == nil {
		opener = newDefaultFileLinkOpener()
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fileLinkOpenTimeout)
		defer cancel()
		err := opener.Open(ctx, resolved)
		return fileLinkOpenResultMsg{target: resolved.OpenTarget(), err: err}
	}
}
