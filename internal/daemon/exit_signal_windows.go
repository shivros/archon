//go:build windows

package daemon

func isExitSignal(err error) bool {
	return false
}
