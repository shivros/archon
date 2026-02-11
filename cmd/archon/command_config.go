package main

import (
	"encoding/json"
	"errors"
	"flag"
	"io"
	"strings"

	"control/internal/app"
	"control/internal/config"

	toml "github.com/pelletier/go-toml/v2"
)

type ConfigCommand struct {
	stdout io.Writer
	stderr io.Writer
}

const (
	configFormatJSON = "json"
	configFormatTOML = "toml"

	configScopeCore        = "core"
	configScopeUI          = "ui"
	configScopeKeybindings = "keybindings"
)

type configOutput struct {
	CoreConfigPath  string                    `json:"core_config_path,omitempty" toml:"core_config_path,omitempty"`
	UIConfigPath    string                    `json:"ui_config_path,omitempty" toml:"ui_config_path,omitempty"`
	KeybindingsPath string                    `json:"keybindings_path,omitempty" toml:"keybindings_path,omitempty"`
	Daemon          *effectiveDaemonConfig    `json:"daemon,omitempty" toml:"daemon,omitempty"`
	Logging         *effectiveLoggingConfig   `json:"logging,omitempty" toml:"logging,omitempty"`
	Debug           *effectiveDebugConfig     `json:"debug,omitempty" toml:"debug,omitempty"`
	Providers       *effectiveProvidersConfig `json:"providers,omitempty" toml:"providers,omitempty"`
	Keybindings     map[string]string         `json:"keybindings,omitempty" toml:"keybindings,omitempty"`
}

type coreConfigOutput struct {
	Daemon    coreDaemonConfigOut      `json:"daemon" toml:"daemon"`
	Logging   effectiveLoggingConfig   `json:"logging" toml:"logging"`
	Debug     effectiveDebugConfig     `json:"debug" toml:"debug"`
	Providers effectiveProvidersConfig `json:"providers" toml:"providers"`
}

type coreDaemonConfigOut struct {
	Address string `json:"address" toml:"address"`
}

type uiConfigOutput struct {
	Keybindings uiKeybindingsConfigOutput `json:"keybindings" toml:"keybindings"`
}

type uiKeybindingsConfigOutput struct {
	Path string `json:"path,omitempty" toml:"path,omitempty"`
}

type effectiveDaemonConfig struct {
	Address string `json:"address" toml:"address"`
	BaseURL string `json:"base_url" toml:"base_url"`
}

type effectiveLoggingConfig struct {
	Level string `json:"level" toml:"level"`
}

type effectiveDebugConfig struct {
	StreamDebug bool `json:"stream_debug" toml:"stream_debug"`
}

type effectiveProvidersConfig struct {
	Codex    effectiveCodexProviderConfig   `json:"codex" toml:"codex"`
	Claude   effectiveClaudeProviderConfig  `json:"claude" toml:"claude"`
	OpenCode effectiveCommandProviderConfig `json:"opencode" toml:"opencode"`
	Gemini   effectiveCommandProviderConfig `json:"gemini" toml:"gemini"`
}

type effectiveCommandProviderConfig struct {
	Command string `json:"command,omitempty" toml:"command,omitempty"`
}

type effectiveCodexProviderConfig struct {
	Command        string   `json:"command,omitempty" toml:"command,omitempty"`
	DefaultModel   string   `json:"default_model" toml:"default_model"`
	Models         []string `json:"models" toml:"models"`
	ApprovalPolicy string   `json:"approval_policy,omitempty" toml:"approval_policy,omitempty"`
	SandboxPolicy  string   `json:"sandbox_policy,omitempty" toml:"sandbox_policy,omitempty"`
	NetworkAccess  *bool    `json:"network_access,omitempty" toml:"network_access,omitempty"`
}

type effectiveClaudeProviderConfig struct {
	Command        string   `json:"command,omitempty" toml:"command,omitempty"`
	DefaultModel   string   `json:"default_model" toml:"default_model"`
	Models         []string `json:"models" toml:"models"`
	IncludePartial bool     `json:"include_partial" toml:"include_partial"`
}

func NewConfigCommand(stdout, stderr io.Writer) *ConfigCommand {
	return &ConfigCommand{
		stdout: stdout,
		stderr: stderr,
	}
}

func (c *ConfigCommand) Run(args []string) error {
	fs := flag.NewFlagSet("config", flag.ContinueOnError)
	fs.SetOutput(c.stderr)
	defaults := fs.Bool("default", false, "print default config values")
	format := fs.String("format", configFormatJSON, "output format: json|toml")
	var scopes stringList
	fs.Var(&scopes, "scope", "scope to print: core|ui|keybindings|all (repeatable)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	resolvedFormat, err := resolveConfigFormat(*format)
	if err != nil {
		return err
	}
	resolvedScopes, err := resolveConfigScopes(scopes)
	if err != nil {
		return err
	}
	payload, err := c.buildOutput(*defaults, resolvedScopes)
	if err != nil {
		return err
	}
	return writeConfigOutput(c.stdout, resolvedFormat, projectedConfigPayload(payload, resolvedScopes))
}

func (c *ConfigCommand) buildOutput(defaults bool, scopes map[string]struct{}) (configOutput, error) {
	out := configOutput{}

	includeCore := scopeSelected(scopes, configScopeCore)
	includeUI := scopeSelected(scopes, configScopeUI)
	includeKeybindings := scopeSelected(scopes, configScopeKeybindings)

	var uiCfg config.UIConfig
	var keybindingsPath string
	if includeUI || includeKeybindings {
		uiPath, err := config.UIConfigPath()
		if err != nil {
			return configOutput{}, err
		}
		if defaults {
			uiCfg = config.DefaultUIConfig()
		} else {
			uiCfg, err = config.LoadUIConfig()
			if err != nil {
				return configOutput{}, err
			}
		}
		keybindingsPath, err = uiCfg.ResolveKeybindingsPath()
		if err != nil {
			return configOutput{}, err
		}
		if includeUI {
			out.UIConfigPath = uiPath
			out.KeybindingsPath = keybindingsPath
		}
	}

	if includeCore {
		corePath, err := config.CoreConfigPath()
		if err != nil {
			return configOutput{}, err
		}
		var coreCfg config.CoreConfig
		if defaults {
			coreCfg = config.DefaultCoreConfig()
		} else {
			coreCfg, err = config.LoadCoreConfig()
			if err != nil {
				return configOutput{}, err
			}
		}
		var networkAccess *bool
		if value, ok := coreCfg.CodexNetworkAccess(); ok {
			networkAccess = &value
		}
		out.CoreConfigPath = corePath
		out.Daemon = &effectiveDaemonConfig{
			Address: coreCfg.DaemonAddress(),
			BaseURL: coreCfg.DaemonBaseURL(),
		}
		out.Logging = &effectiveLoggingConfig{
			Level: coreCfg.LogLevel(),
		}
		out.Debug = &effectiveDebugConfig{
			StreamDebug: coreCfg.StreamDebugEnabled(),
		}
		out.Providers = &effectiveProvidersConfig{
			Codex: effectiveCodexProviderConfig{
				Command:        coreCfg.ProviderCommand("codex"),
				DefaultModel:   coreCfg.CodexDefaultModel(),
				Models:         coreCfg.CodexModels(),
				ApprovalPolicy: coreCfg.CodexApprovalPolicy(),
				SandboxPolicy:  coreCfg.CodexSandboxPolicy(),
				NetworkAccess:  networkAccess,
			},
			Claude: effectiveClaudeProviderConfig{
				Command:        coreCfg.ProviderCommand("claude"),
				DefaultModel:   coreCfg.ClaudeDefaultModel(),
				Models:         coreCfg.ClaudeModels(),
				IncludePartial: coreCfg.ClaudeIncludePartial(),
			},
			OpenCode: effectiveCommandProviderConfig{
				Command: coreCfg.ProviderCommand("opencode"),
			},
			Gemini: effectiveCommandProviderConfig{
				Command: coreCfg.ProviderCommand("gemini"),
			},
		}
	}

	if includeKeybindings {
		var bindings *app.Keybindings
		var err error
		if defaults {
			bindings = app.DefaultKeybindings()
		} else {
			bindings, err = app.LoadKeybindings(keybindingsPath)
			if err != nil {
				return configOutput{}, err
			}
		}
		out.Keybindings = bindings.Bindings()
	}

	return out, nil
}

func writeConfigOutput(out io.Writer, format string, payload any) error {
	switch format {
	case configFormatJSON:
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(payload)
	case configFormatTOML:
		data, err := toml.Marshal(payload)
		if err != nil {
			return err
		}
		if len(data) == 0 || data[len(data)-1] != '\n' {
			data = append(data, '\n')
		}
		_, err = out.Write(data)
		return err
	default:
		return errors.New("unsupported format")
	}
}

func projectedConfigPayload(payload configOutput, scopes map[string]struct{}) any {
	if len(scopes) != 1 {
		return payload
	}
	if scopeSelected(scopes, configScopeKeybindings) {
		if payload.Keybindings == nil {
			return map[string]string{}
		}
		return payload.Keybindings
	}
	if scopeSelected(scopes, configScopeUI) {
		return uiConfigOutput{
			Keybindings: uiKeybindingsConfigOutput{
				Path: payload.KeybindingsPath,
			},
		}
	}
	if scopeSelected(scopes, configScopeCore) {
		out := coreConfigOutput{
			Daemon: coreDaemonConfigOut{},
			Logging: effectiveLoggingConfig{
				Level: "info",
			},
			Debug: effectiveDebugConfig{
				StreamDebug: false,
			},
		}
		if payload.Daemon != nil {
			out.Daemon.Address = payload.Daemon.Address
		}
		if payload.Logging != nil {
			out.Logging = *payload.Logging
		}
		if payload.Debug != nil {
			out.Debug = *payload.Debug
		}
		if payload.Providers != nil {
			out.Providers = *payload.Providers
		}
		return out
	}
	return payload
}

func resolveConfigFormat(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", configFormatJSON:
		return configFormatJSON, nil
	case configFormatTOML:
		return configFormatTOML, nil
	default:
		return "", errors.New("invalid format: must be json or toml")
	}
}

func resolveConfigScopes(values []string) (map[string]struct{}, error) {
	if len(values) == 0 {
		return map[string]struct{}{
			configScopeCore:        {},
			configScopeUI:          {},
			configScopeKeybindings: {},
		}, nil
	}
	out := map[string]struct{}{}
	for _, raw := range values {
		parts := strings.Split(raw, ",")
		for _, part := range parts {
			scope, err := normalizeConfigScope(part)
			if err != nil {
				return nil, err
			}
			if scope == "all" {
				return map[string]struct{}{
					configScopeCore:        {},
					configScopeUI:          {},
					configScopeKeybindings: {},
				}, nil
			}
			out[scope] = struct{}{}
		}
	}
	return out, nil
}

func normalizeConfigScope(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "all":
		return "all", nil
	case configScopeCore, "daemon":
		return configScopeCore, nil
	case configScopeUI:
		return configScopeUI, nil
	case configScopeKeybindings, "keys":
		return configScopeKeybindings, nil
	default:
		return "", errors.New("invalid scope: must be core, ui, keybindings, or all")
	}
}

func scopeSelected(scopes map[string]struct{}, scope string) bool {
	_, ok := scopes[scope]
	return ok
}
