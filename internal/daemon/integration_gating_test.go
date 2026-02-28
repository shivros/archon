package daemon

import (
	"os"
	"strings"
)

func integrationEnvDisabled(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "0", "false", "off", "no", "disabled":
		return true
	default:
		return false
	}
}
