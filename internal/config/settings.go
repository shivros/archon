package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

const defaultDaemonAddress = "127.0.0.1:7777"

type CoreConfig struct {
	Daemon CoreDaemonConfig `toml:"daemon"`
}

type CoreDaemonConfig struct {
	Address string `toml:"address"`
}

type UIConfig struct {
	Keybindings UIKeybindingsConfig `toml:"keybindings"`
}

type UIKeybindingsConfig struct {
	Path string `toml:"path"`
}

func DefaultCoreConfig() CoreConfig {
	return CoreConfig{
		Daemon: CoreDaemonConfig{
			Address: defaultDaemonAddress,
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

func DefaultUIConfig() UIConfig {
	return UIConfig{}
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
