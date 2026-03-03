package app

import (
	"fmt"
	"strings"
	"time"

	"control/internal/daemon/transcriptdomain"
	"control/internal/types"
)

type sessionSelectionSnapshot struct {
	key       string
	sessionID string
	revision  string
	mode      sessionSemanticMode
	isSession bool
}

type SessionSelectionLoadPolicy interface {
	SelectionLoadDelay(base time.Duration) time.Duration
}

type defaultSessionSelectionLoadPolicy struct{}

func WithSessionSelectionLoadPolicy(policy SessionSelectionLoadPolicy) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if policy == nil {
			m.selectionLoadPolicy = defaultSessionSelectionLoadPolicy{}
			return
		}
		m.selectionLoadPolicy = policy
	}
}

func (defaultSessionSelectionLoadPolicy) SelectionLoadDelay(base time.Duration) time.Duration {
	if base < 0 {
		return 0
	}
	return base
}

func (m *Model) selectionLoadPolicyOrDefault() SessionSelectionLoadPolicy {
	if m == nil || m.selectionLoadPolicy == nil {
		return defaultSessionSelectionLoadPolicy{}
	}
	return m.selectionLoadPolicy
}

func (m *Model) selectedSessionSnapshot() sessionSelectionSnapshot {
	if m == nil {
		return sessionSelectionSnapshot{}
	}
	item := m.selectedItem()
	if item == nil || item.session == nil {
		return sessionSelectionSnapshot{}
	}
	id := strings.TrimSpace(item.session.ID)
	if id == "" {
		return sessionSelectionSnapshot{}
	}
	var capabilities *transcriptdomain.CapabilityEnvelope
	if value, ok := m.sessionTranscriptCapabilitiesForSession(id); ok {
		capabilities = value
	}
	return sessionSelectionSnapshot{
		key:       item.key(),
		sessionID: id,
		revision:  sessionRevision(item.session, m.sessionMeta[id]),
		mode:      m.sessionCapabilityModeResolverOrDefault().ResolveMode(id, item.session.Provider, capabilities),
		isSession: true,
	}
}

func sessionRevision(session *types.Session, meta *types.SessionMeta) string {
	if session == nil {
		return ""
	}
	exitCode := ""
	if session.ExitCode != nil {
		exitCode = fmt.Sprintf("%d", *session.ExitCode)
	}
	exitedAt := ""
	if session.ExitedAt != nil {
		exitedAt = session.ExitedAt.UTC().Format(time.RFC3339Nano)
	}
	startedAt := ""
	if session.StartedAt != nil {
		startedAt = session.StartedAt.UTC().Format(time.RFC3339Nano)
	}
	metaDismissed := ""
	if meta != nil && meta.DismissedAt != nil {
		metaDismissed = meta.DismissedAt.UTC().Format(time.RFC3339Nano)
	}
	metaRuntime := runtimeOptionsRevision(nil)
	if meta != nil {
		metaRuntime = runtimeOptionsRevision(meta.RuntimeOptions)
	}
	metaThreadID := ""
	metaProviderSessionID := ""
	if meta != nil {
		metaThreadID = strings.TrimSpace(meta.ThreadID)
		metaProviderSessionID = strings.TrimSpace(meta.ProviderSessionID)
	}
	return strings.Join([]string{
		strings.TrimSpace(session.ID),
		strings.TrimSpace(session.Provider),
		session.CreatedAt.UTC().Format(time.RFC3339Nano),
		startedAt,
		exitedAt,
		exitCode,
		metaThreadID,
		metaProviderSessionID,
		metaDismissed,
		metaRuntime,
	}, "|")
}

func runtimeOptionsRevision(options *types.SessionRuntimeOptions) string {
	if options == nil {
		return ""
	}
	return strings.Join([]string{
		strings.TrimSpace(options.Model),
		string(options.Reasoning),
		string(options.Access),
		fmt.Sprintf("%d", options.Version),
	}, "|")
}
