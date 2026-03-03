package app

import "strings"

type sessionSemanticMode struct {
	SupportsApprovals bool
	SupportsEvents    bool
	UsesItems         bool
	SupportsInterrupt bool
	NoProcess         bool
}

func (m sessionSemanticMode) Equal(other sessionSemanticMode) bool {
	return m == other
}

type sessionReloadDecision struct {
	Reload bool
	Reason string
}

type SessionReloadDecisionPolicy interface {
	DecideReload(previous, next sessionSelectionSnapshot) sessionReloadDecision
}

type defaultSessionReloadDecisionPolicy struct{}

func WithSessionReloadDecisionPolicy(policy SessionReloadDecisionPolicy) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if policy == nil {
			m.sessionReloadPolicy = defaultSessionReloadDecisionPolicy{}
			return
		}
		m.sessionReloadPolicy = policy
	}
}

func (defaultSessionReloadDecisionPolicy) DecideReload(previous, next sessionSelectionSnapshot) sessionReloadDecision {
	if !next.isSession {
		return sessionReloadDecision{Reason: transcriptReasonNotSessionSelection}
	}
	if !previous.isSession {
		return sessionReloadDecision{Reload: true, Reason: transcriptReasonSelectedSessionFromNon}
	}
	if previous.sessionID != next.sessionID {
		return sessionReloadDecision{Reload: true, Reason: transcriptReasonSelectedSessionChanged}
	}
	if previous.key != next.key {
		return sessionReloadDecision{Reload: true, Reason: transcriptReasonSelectedKeyChanged}
	}
	if !previous.mode.Equal(next.mode) {
		return sessionReloadDecision{Reload: true, Reason: transcriptReasonReloadSemanticCapabilityChanged}
	}
	if previous.revision != next.revision {
		return sessionReloadDecision{Reload: true, Reason: transcriptReasonSelectedRevisionChanged}
	}
	return sessionReloadDecision{
		Reload: false,
		Reason: transcriptReasonReloadVolatileMetadataIgnored,
	}
}

func (m *Model) sessionReloadPolicyOrDefault() SessionReloadDecisionPolicy {
	if m == nil || m.sessionReloadPolicy == nil {
		return defaultSessionReloadDecisionPolicy{}
	}
	return m.sessionReloadPolicy
}

func coalesceKeyForSelection(snapshot sessionSelectionSnapshot) string {
	if !snapshot.isSession {
		return ""
	}
	sessionID := strings.TrimSpace(snapshot.sessionID)
	if sessionID == "" {
		return ""
	}
	return sessionID + "|" + strings.TrimSpace(snapshot.revision) + "|" + semanticModeKey(snapshot.mode)
}

func semanticModeKey(mode sessionSemanticMode) string {
	var b strings.Builder
	if mode.SupportsApprovals {
		b.WriteString("a1")
	} else {
		b.WriteString("a0")
	}
	if mode.SupportsEvents {
		b.WriteString("|e1")
	} else {
		b.WriteString("|e0")
	}
	if mode.UsesItems {
		b.WriteString("|i1")
	} else {
		b.WriteString("|i0")
	}
	if mode.SupportsInterrupt {
		b.WriteString("|x1")
	} else {
		b.WriteString("|x0")
	}
	if mode.NoProcess {
		b.WriteString("|n1")
	} else {
		b.WriteString("|n0")
	}
	return b.String()
}
