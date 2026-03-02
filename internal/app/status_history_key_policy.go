package app

type statusHistoryKeyAction int

const (
	statusHistoryKeyActionClose statusHistoryKeyAction = iota
	statusHistoryKeyActionMoveUp
	statusHistoryKeyActionMoveDown
	statusHistoryKeyActionPageUp
	statusHistoryKeyActionPageDown
	statusHistoryKeyActionHome
	statusHistoryKeyActionEnd
	statusHistoryKeyActionCopy
)

type StatusHistoryKeyPolicy interface {
	ActionFor(key string) (statusHistoryKeyAction, bool)
}

type defaultStatusHistoryKeyPolicy struct{}

func (defaultStatusHistoryKeyPolicy) ActionFor(key string) (statusHistoryKeyAction, bool) {
	switch key {
	case "esc":
		return statusHistoryKeyActionClose, true
	case "up", "k":
		return statusHistoryKeyActionMoveUp, true
	case "down", "j":
		return statusHistoryKeyActionMoveDown, true
	case "pgup":
		return statusHistoryKeyActionPageUp, true
	case "pgdown":
		return statusHistoryKeyActionPageDown, true
	case "home":
		return statusHistoryKeyActionHome, true
	case "end":
		return statusHistoryKeyActionEnd, true
	case "enter", "c":
		return statusHistoryKeyActionCopy, true
	default:
		return statusHistoryKeyActionClose, false
	}
}
