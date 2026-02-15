package app

import (
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

type toastLevel int

const (
	toastLevelInfo toastLevel = iota
	toastLevelWarning
	toastLevelError
)

type queuedToast struct {
	level   toastLevel
	message string
}

func (m *Model) showInfoToast(message string) {
	m.showToast(toastLevelInfo, message)
}

func (m *Model) showWarningToast(message string) {
	m.showToast(toastLevelWarning, message)
}

func (m *Model) showErrorToast(message string) {
	m.showToast(toastLevelError, message)
}

func (m *Model) showToast(level toastLevel, message string) {
	message = strings.TrimSpace(message)
	if message == "" {
		return
	}
	m.toastText = message
	m.toastLevel = level
	m.toastUntil = time.Now().Add(toastDuration)
}

func (m *Model) clearToast() {
	m.toastText = ""
	m.toastLevel = toastLevelInfo
	m.toastUntil = time.Time{}
}

func (m *Model) enqueueStartupToast(level toastLevel, message string) {
	message = strings.TrimSpace(message)
	if message == "" {
		return
	}
	m.startupToasts = append(m.startupToasts, queuedToast{level: level, message: message})
	m.maybeShowNextStartupToast(time.Now())
}

func (m *Model) maybeShowNextStartupToast(at time.Time) {
	if len(m.startupToasts) == 0 {
		return
	}
	if m.toastActive(at) {
		return
	}
	next := m.startupToasts[0]
	m.startupToasts = m.startupToasts[1:]
	m.status = next.message
	m.showToast(next.level, next.message)
}

func (m *Model) toastActive(at time.Time) bool {
	if strings.TrimSpace(m.toastText) == "" {
		return false
	}
	if m.toastUntil.IsZero() {
		return true
	}
	if at.IsZero() {
		at = time.Now()
	}
	return at.Before(m.toastUntil)
}

func (m *Model) toastLine(width int) string {
	if !m.toastActive(time.Now()) || width <= 0 {
		return ""
	}
	maxTextWidth := max(1, width-4)
	text := truncateToWidth(m.toastText, maxTextWidth)
	pill := m.toastStyle().Render(" " + text + " ")
	return lipgloss.PlaceHorizontal(width, lipgloss.Right, pill)
}

func (m *Model) toastStyle() lipgloss.Style {
	switch m.toastLevel {
	case toastLevelWarning:
		return toastWarningStyle
	case toastLevelError:
		return toastErrorStyle
	default:
		return toastInfoStyle
	}
}
