package daemon

import (
	"os"
	"path/filepath"
	"strings"
)

const codexHomeDirName = ".archon"

func resolveCodexHome(cwd, workspacePath string) string {
	trimmed := strings.TrimSpace(cwd)
	if trimmed != "" {
		candidate := filepath.Join(trimmed, codexHomeDirName)
		if pathExists(candidate) {
			return candidate
		}
	}
	workspaceTrimmed := strings.TrimSpace(workspacePath)
	if workspaceTrimmed == "" || workspaceTrimmed == trimmed {
		return ""
	}
	candidate := filepath.Join(workspaceTrimmed, codexHomeDirName)
	if pathExists(candidate) {
		return candidate
	}
	return ""
}

func pathExists(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}
