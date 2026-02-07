package testutil

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"control/internal/config"
)

type codexKeysFile struct {
	CodexAPIKey  string `json:"codex_api_key"`
	OpenAIAPIKey string `json:"openai_api_key"`
}

// LoadCodexAPIKey returns an API key for Codex integration tests.
// Lookup order:
// 1) ARCHON_CODEX_API_KEY
// 2) OPENAI_API_KEY
// 3) ~/.archon/test-keys.json (codex_api_key or openai_api_key)
// 4) ~/.archon/codex_api_key (raw)
// 5) ~/.archon/openai_api_key (raw)
func LoadCodexAPIKey() string {
	if key := strings.TrimSpace(os.Getenv("ARCHON_CODEX_API_KEY")); key != "" {
		return key
	}
	if key := strings.TrimSpace(os.Getenv("OPENAI_API_KEY")); key != "" {
		return key
	}
	if key := readCodexKeysJSON(); key != "" {
		return key
	}
	if key := readKeyFile("codex_api_key"); key != "" {
		return key
	}
	if key := readKeyFile("openai_api_key"); key != "" {
		return key
	}
	return ""
}

func readCodexKeysJSON() string {
	dataDir, err := config.DataDir()
	if err != nil {
		return ""
	}
	path := filepath.Join(dataDir, "test-keys.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var parsed codexKeysFile
	if err := json.Unmarshal(data, &parsed); err != nil {
		return ""
	}
	if key := strings.TrimSpace(parsed.CodexAPIKey); key != "" {
		return key
	}
	if key := strings.TrimSpace(parsed.OpenAIAPIKey); key != "" {
		return key
	}
	return ""
}

func readKeyFile(name string) string {
	dataDir, err := config.DataDir()
	if err != nil {
		return ""
	}
	path := filepath.Join(dataDir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
