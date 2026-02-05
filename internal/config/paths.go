package config

import (
	"os"
	"path/filepath"
)

const appDirName = ".control"

// DataDir returns the base data directory for Control.
func DataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, appDirName), nil
}

// SessionsDir returns the directory where session files are stored.
func SessionsDir() (string, error) {
	dataDir, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dataDir, "sessions"), nil
}

// TokenPath returns the path to the token file.
func TokenPath() (string, error) {
	dataDir, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dataDir, "token"), nil
}

// WorkspacesPath returns the path to the workspace metadata file.
func WorkspacesPath() (string, error) {
	dataDir, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dataDir, "workspaces.json"), nil
}

// StatePath returns the path to the persisted UI state file.
func StatePath() (string, error) {
	dataDir, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dataDir, "state.json"), nil
}

// KeymapPath returns the path to the keymap configuration file.
func KeymapPath() (string, error) {
	dataDir, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dataDir, "keymap.json"), nil
}

// SessionsMetaPath returns the path to the session metadata file.
func SessionsMetaPath() (string, error) {
	dataDir, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dataDir, "sessions_meta.json"), nil
}

// SessionsIndexPath returns the path to the session index file.
func SessionsIndexPath() (string, error) {
	dataDir, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dataDir, "sessions_index.json"), nil
}

// ApprovalsPath returns the path to the approvals file.
func ApprovalsPath() (string, error) {
	dataDir, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dataDir, "approvals.json"), nil
}
