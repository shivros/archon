package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"control/internal/config"
)

type fileSessionItemsRepository struct {
	baseDirResolver func() (string, error)
	broadcastItems  func(sessionID string, items []map[string]any)
}

func newFileSessionItemsRepository(manager *SessionManager) TurnArtifactRepository {
	return &fileSessionItemsRepository{
		baseDirResolver: func() (string, error) {
			if manager != nil {
				if baseDir := strings.TrimSpace(manager.SessionsBaseDir()); baseDir != "" {
					return baseDir, nil
				}
			}
			return config.SessionsDir()
		},
		broadcastItems: func(sessionID string, items []map[string]any) {
			if manager == nil {
				return
			}
			manager.BroadcastItems(sessionID, items)
		},
	}
}

func (r *fileSessionItemsRepository) ReadItems(sessionID string, lines int) ([]map[string]any, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, nil
	}
	if lines <= 0 {
		lines = 200
	}
	baseDir, err := r.baseDir()
	if err != nil {
		return nil, err
	}
	itemsPath := filepath.Join(baseDir, sessionID, "items.jsonl")
	if _, err := os.Stat(itemsPath); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	rawLines, _, err := tailLines(itemsPath, lines)
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(rawLines))
	for _, line := range rawLines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(line), &payload); err != nil {
			continue
		}
		if payload != nil {
			items = append(items, payload)
		}
	}
	return items, nil
}

func (r *fileSessionItemsRepository) AppendItems(sessionID string, items []map[string]any) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || len(items) == 0 {
		return nil
	}
	baseDir, err := r.baseDir()
	if err != nil {
		return err
	}
	sessionDir := filepath.Join(baseDir, sessionID)
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		return err
	}
	itemsPath := filepath.Join(sessionDir, "items.jsonl")
	file, err := os.OpenFile(itemsPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	appended := make([]map[string]any, 0, len(items))
	for _, item := range items {
		prepared := prepareItemForPersistence(item, time.Now().UTC())
		if prepared == nil {
			continue
		}
		data, err := json.Marshal(prepared)
		if err != nil {
			continue
		}
		if _, err := file.Write(append(data, '\n')); err != nil {
			return err
		}
		appended = append(appended, prepared)
	}
	if len(appended) > 0 && r.broadcastItems != nil {
		r.broadcastItems(sessionID, appended)
	}
	return nil
}

func (r *fileSessionItemsRepository) baseDir() (string, error) {
	if r == nil || r.baseDirResolver == nil {
		return config.SessionsDir()
	}
	return r.baseDirResolver()
}
