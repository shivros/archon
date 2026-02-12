package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

const defaultDaemonAddress = "127.0.0.1:7777"
const (
	defaultCodexModel  = "gpt-5.1-codex"
	defaultClaudeModel = "sonnet"
)

var defaultCodexModels = []string{
	"gpt-5.1-codex",
	"gpt-5.2-codex",
	"gpt-5.3-codex",
	"gpt-5.1-codex-max",
}

var defaultClaudeModels = []string{"sonnet", "opus"}

type CoreConfig struct {
	Daemon    CoreDaemonConfig    `toml:"daemon"`
	Providers CoreProvidersConfig `toml:"providers"`
	Logging   CoreLoggingConfig   `toml:"logging"`
	Debug     CoreDebugConfig     `toml:"debug"`
}

type CoreDaemonConfig struct {
	Address string `toml:"address"`
}

type CoreLoggingConfig struct {
	Level string `toml:"level"`
}

type CoreDebugConfig struct {
	StreamDebug bool `toml:"stream_debug"`
}

type CoreProvidersConfig struct {
	Codex    CoreCodexProviderConfig   `toml:"codex"`
	Claude   CoreClaudeProviderConfig  `toml:"claude"`
	OpenCode CoreCommandProviderConfig `toml:"opencode"`
	Gemini   CoreCommandProviderConfig `toml:"gemini"`
}

type CoreCommandProviderConfig struct {
	Command string `toml:"command"`
}

type CoreCodexProviderConfig struct {
	Command        string   `toml:"command"`
	DefaultModel   string   `toml:"default_model"`
	Models         []string `toml:"models"`
	ApprovalPolicy string   `toml:"approval_policy"`
	SandboxPolicy  string   `toml:"sandbox_policy"`
	NetworkAccess  *bool    `toml:"network_access"`
}

type CoreClaudeProviderConfig struct {
	Command        string   `toml:"command"`
	DefaultModel   string   `toml:"default_model"`
	Models         []string `toml:"models"`
	IncludePartial bool     `toml:"include_partial"`
}

type UIConfig struct {
	Keybindings UIKeybindingsConfig `toml:"keybindings"`
	Input       UIInputConfig       `toml:"input"`
}

type UIKeybindingsConfig struct {
	Path string `toml:"path"`
}

type UIInputConfig struct {
	MultilineMinHeight int `toml:"multiline_min_height"`
	MultilineMaxHeight int `toml:"multiline_max_height"`
}

func DefaultCoreConfig() CoreConfig {
	return CoreConfig{
		Daemon: CoreDaemonConfig{
			Address: defaultDaemonAddress,
		},
		Logging: CoreLoggingConfig{
			Level: "info",
		},
		Providers: CoreProvidersConfig{
			Codex: CoreCodexProviderConfig{
				DefaultModel: defaultCodexModel,
				Models:       append([]string{}, defaultCodexModels...),
			},
			Claude: CoreClaudeProviderConfig{
				DefaultModel: defaultClaudeModel,
				Models:       append([]string{}, defaultClaudeModels...),
			},
		},
	}
}

func LoadCoreConfig() (CoreConfig, error) {
	path, err := CoreConfigPath()
	if err != nil {
		return CoreConfig{}, err
	}
	return loadCoreConfigFromPath(path)
}

func (c CoreConfig) DaemonAddress() string {
	addr := strings.TrimSpace(c.Daemon.Address)
	if addr == "" {
		return defaultDaemonAddress
	}
	addr = strings.TrimPrefix(addr, "http://")
	addr = strings.TrimPrefix(addr, "https://")
	addr = strings.TrimRight(addr, "/")
	if addr == "" {
		return defaultDaemonAddress
	}
	return addr
}

func (c CoreConfig) DaemonBaseURL() string {
	addr := strings.TrimSpace(c.DaemonAddress())
	return "http://" + addr
}

func (c CoreConfig) LogLevel() string {
	level := strings.TrimSpace(c.Logging.Level)
	if level == "" {
		return "info"
	}
	return level
}

func (c CoreConfig) StreamDebugEnabled() bool {
	return c.Debug.StreamDebug
}

func (c CoreConfig) ProviderCommand(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "codex":
		return strings.TrimSpace(c.Providers.Codex.Command)
	case "claude":
		return strings.TrimSpace(c.Providers.Claude.Command)
	case "opencode":
		return strings.TrimSpace(c.Providers.OpenCode.Command)
	case "gemini":
		return strings.TrimSpace(c.Providers.Gemini.Command)
	default:
		return ""
	}
}

func (c CoreConfig) CodexDefaultModel() string {
	model := strings.TrimSpace(c.Providers.Codex.DefaultModel)
	if model == "" {
		return defaultCodexModel
	}
	return model
}

func (c CoreConfig) CodexModels() []string {
	models := normalizedList(c.Providers.Codex.Models)
	if len(models) == 0 {
		models = append([]string{}, defaultCodexModels...)
	}
	return models
}

func (c CoreConfig) ClaudeDefaultModel() string {
	model := strings.TrimSpace(c.Providers.Claude.DefaultModel)
	if model == "" {
		return defaultClaudeModel
	}
	return model
}

func (c CoreConfig) ClaudeModels() []string {
	models := normalizedList(c.Providers.Claude.Models)
	if len(models) == 0 {
		models = append([]string{}, defaultClaudeModels...)
	}
	return models
}

func (c CoreConfig) ClaudeIncludePartial() bool {
	return c.Providers.Claude.IncludePartial
}

func (c CoreConfig) CodexApprovalPolicy() string {
	return strings.TrimSpace(c.Providers.Codex.ApprovalPolicy)
}

func (c CoreConfig) CodexSandboxPolicy() string {
	return strings.TrimSpace(c.Providers.Codex.SandboxPolicy)
}

func (c CoreConfig) CodexNetworkAccess() (bool, bool) {
	if c.Providers.Codex.NetworkAccess == nil {
		return false, false
	}
	return *c.Providers.Codex.NetworkAccess, true
}

func DefaultUIConfig() UIConfig {
	return UIConfig{
		Input: UIInputConfig{
			MultilineMinHeight: 3,
			MultilineMaxHeight: 8,
		},
	}
}

func (c UIConfig) SharedMultilineInputHeights() (minHeight, maxHeight int) {
	minHeight = c.Input.MultilineMinHeight
	maxHeight = c.Input.MultilineMaxHeight
	if minHeight <= 0 {
		minHeight = 3
	}
	if maxHeight <= 0 {
		maxHeight = 8
	}
	if maxHeight < minHeight {
		maxHeight = minHeight
	}
	return minHeight, maxHeight
}

func LoadUIConfig() (UIConfig, error) {
	path, err := UIConfigPath()
	if err != nil {
		return UIConfig{}, err
	}
	return loadUIConfigFromPath(path)
}

func (c UIConfig) ResolveKeybindingsPath() (string, error) {
	defaultPath, err := KeybindingsPath()
	if err != nil {
		return "", err
	}
	path := strings.TrimSpace(c.Keybindings.Path)
	if path == "" {
		return defaultPath, nil
	}
	path, err = resolveConfigPath(path)
	if err != nil {
		return "", err
	}
	return path, nil
}

func loadCoreConfigFromPath(path string) (CoreConfig, error) {
	cfg := DefaultCoreConfig()
	if err := readTOML(path, &cfg); err != nil {
		return CoreConfig{}, err
	}
	return cfg, nil
}

func loadUIConfigFromPath(path string) (UIConfig, error) {
	cfg := DefaultUIConfig()
	if err := readTOML(path, &cfg); err != nil {
		return UIConfig{}, err
	}
	return cfg, nil
}

func readTOML(path string, out any) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("path is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil
	}
	return toml.Unmarshal(data, out)
}

func resolveConfigPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("path is required")
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, path[2:]), nil
	}
	if filepath.IsAbs(path) {
		return path, nil
	}
	dataDir, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dataDir, path), nil
}

func normalizedList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
