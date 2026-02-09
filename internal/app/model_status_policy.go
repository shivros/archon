package app

import "strings"

type statusEvent int

const (
	statusEventStatusOnly statusEvent = iota
	statusEventValidationWarning
	statusEventBackgroundInfo
	statusEventBackgroundError
	statusEventApprovalWarning
	statusEventCopyInfo
	statusEventCopyWarning
	statusEventCopyError
	statusEventActionInfo
	statusEventActionWarning
	statusEventActionError
)

type statusPolicy struct {
	showToast bool
	level     toastLevel
}

func statusPolicyForEvent(event statusEvent) statusPolicy {
	switch event {
	case statusEventValidationWarning:
		return statusPolicy{showToast: true, level: toastLevelWarning}
	case statusEventBackgroundInfo:
		return statusPolicy{showToast: false, level: toastLevelInfo}
	case statusEventBackgroundError:
		return statusPolicy{showToast: true, level: toastLevelError}
	case statusEventApprovalWarning:
		return statusPolicy{showToast: true, level: toastLevelWarning}
	case statusEventCopyInfo:
		return statusPolicy{showToast: true, level: toastLevelInfo}
	case statusEventCopyWarning:
		return statusPolicy{showToast: true, level: toastLevelWarning}
	case statusEventCopyError:
		return statusPolicy{showToast: true, level: toastLevelError}
	case statusEventActionInfo:
		return statusPolicy{showToast: true, level: toastLevelInfo}
	case statusEventActionWarning:
		return statusPolicy{showToast: true, level: toastLevelWarning}
	case statusEventActionError:
		return statusPolicy{showToast: true, level: toastLevelError}
	default:
		return statusPolicy{showToast: false, level: toastLevelInfo}
	}
}

func (m *Model) setStatusByEvent(event statusEvent, message string) {
	message = strings.TrimSpace(message)
	if message == "" {
		return
	}
	m.status = message
	policy := statusPolicyForEvent(event)
	if !policy.showToast {
		return
	}
	m.showToast(policy.level, message)
}

func (m *Model) setStatusMessage(message string) {
	m.setStatusByEvent(statusEventStatusOnly, message)
}

func (m *Model) setValidationStatus(message string) {
	m.setStatusByEvent(statusEventValidationWarning, message)
}

func (m *Model) setBackgroundStatus(message string) {
	m.setStatusByEvent(statusEventBackgroundInfo, message)
}

func (m *Model) setBackgroundError(message string) {
	m.setStatusByEvent(statusEventBackgroundError, message)
}

func (m *Model) setApprovalStatus(message string) {
	m.setStatusByEvent(statusEventApprovalWarning, message)
}

func (m *Model) setStatusInfo(message string) {
	m.setStatusByEvent(statusEventActionInfo, message)
}

func (m *Model) setStatusWarning(message string) {
	m.setStatusByEvent(statusEventActionWarning, message)
}

func (m *Model) setStatusError(message string) {
	m.setStatusByEvent(statusEventActionError, message)
}

func (m *Model) setCopyStatusInfo(message string) {
	m.setStatusByEvent(statusEventCopyInfo, message)
}

func (m *Model) setCopyStatusWarning(message string) {
	m.setStatusByEvent(statusEventCopyWarning, message)
}

func (m *Model) setCopyStatusError(message string) {
	m.setStatusByEvent(statusEventCopyError, message)
}
