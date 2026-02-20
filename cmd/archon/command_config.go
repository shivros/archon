package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"strings"

	"control/internal/app"
	"control/internal/config"
	"control/internal/guidedworkflows"
	"control/internal/store"

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
	configScopeWorkflows   = "workflow_templates"
)

type configOutput struct {
	CoreConfigPath        string                          `json:"core_config_path,omitempty" toml:"core_config_path,omitempty"`
	UIConfigPath          string                          `json:"ui_config_path,omitempty" toml:"ui_config_path,omitempty"`
	KeybindingsPath       string                          `json:"keybindings_path,omitempty" toml:"keybindings_path,omitempty"`
	WorkflowTemplatesPath string                          `json:"workflow_templates_path,omitempty" toml:"workflow_templates_path,omitempty"`
	Chat                  *uiChatConfigOutput             `json:"chat,omitempty" toml:"chat,omitempty"`
	Daemon                *effectiveDaemonConfig          `json:"daemon,omitempty" toml:"daemon,omitempty"`
	Logging               *effectiveLoggingConfig         `json:"logging,omitempty" toml:"logging,omitempty"`
	Debug                 *effectiveDebugConfig           `json:"debug,omitempty" toml:"debug,omitempty"`
	Notifications         *effectiveNotificationsConfig   `json:"notifications,omitempty" toml:"notifications,omitempty"`
	GuidedWorkflows       *effectiveGuidedWorkflowsConfig `json:"guided_workflows,omitempty" toml:"guided_workflows,omitempty"`
	WorkflowTemplates     *workflowTemplatesConfigOutput  `json:"workflow_templates,omitempty" toml:"workflow_templates,omitempty"`
	Providers             *effectiveProvidersConfig       `json:"providers,omitempty" toml:"providers,omitempty"`
	Keybindings           map[string]string               `json:"keybindings,omitempty" toml:"keybindings,omitempty"`
}

type coreConfigOutput struct {
	Daemon          coreDaemonConfigOut            `json:"daemon" toml:"daemon"`
	Logging         effectiveLoggingConfig         `json:"logging" toml:"logging"`
	Debug           effectiveDebugConfig           `json:"debug" toml:"debug"`
	Notifications   effectiveNotificationsConfig   `json:"notifications" toml:"notifications"`
	GuidedWorkflows effectiveGuidedWorkflowsConfig `json:"guided_workflows" toml:"guided_workflows"`
	Providers       effectiveProvidersConfig       `json:"providers" toml:"providers"`
}

type coreDaemonConfigOut struct {
	Address string `json:"address" toml:"address"`
}

type uiConfigOutput struct {
	Keybindings uiKeybindingsConfigOutput `json:"keybindings" toml:"keybindings"`
	Chat        uiChatConfigOutput        `json:"chat" toml:"chat"`
}

type uiKeybindingsConfigOutput struct {
	Path string `json:"path,omitempty" toml:"path,omitempty"`
}

type uiChatConfigOutput struct {
	TimestampMode string `json:"timestamp_mode" toml:"timestamp_mode"`
}

type workflowTemplatesConfigOutput struct {
	Templates []guidedworkflows.WorkflowTemplate `json:"templates" toml:"templates"`
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

type effectiveNotificationsConfig struct {
	Enabled              bool     `json:"enabled" toml:"enabled"`
	Triggers             []string `json:"triggers,omitempty" toml:"triggers,omitempty"`
	Methods              []string `json:"methods,omitempty" toml:"methods,omitempty"`
	ScriptCommands       []string `json:"script_commands,omitempty" toml:"script_commands,omitempty"`
	ScriptTimeoutSeconds int      `json:"script_timeout_seconds" toml:"script_timeout_seconds"`
	DedupeWindowSeconds  int      `json:"dedupe_window_seconds" toml:"dedupe_window_seconds"`
}

type effectiveGuidedWorkflowsConfig struct {
	Enabled         bool                                   `json:"enabled" toml:"enabled"`
	AutoStart       bool                                   `json:"auto_start" toml:"auto_start"`
	CheckpointStyle string                                 `json:"checkpoint_style" toml:"checkpoint_style"`
	Mode            string                                 `json:"mode" toml:"mode"`
	Defaults        effectiveGuidedWorkflowsDefaultsConfig `json:"defaults" toml:"defaults"`
	Policy          effectiveGuidedWorkflowsPolicyConfig   `json:"policy" toml:"policy"`
	Rollout         effectiveGuidedWorkflowsRolloutConfig  `json:"rollout" toml:"rollout"`
}

type effectiveGuidedWorkflowsDefaultsConfig struct {
	Provider           string `json:"provider,omitempty" toml:"provider,omitempty"`
	Model              string `json:"model,omitempty" toml:"model,omitempty"`
	Access             string `json:"access,omitempty" toml:"access,omitempty"`
	Reasoning          string `json:"reasoning,omitempty" toml:"reasoning,omitempty"`
	ResolutionBoundary string `json:"resolution_boundary,omitempty" toml:"resolution_boundary,omitempty"`
}

type effectiveGuidedWorkflowsPolicyConfig struct {
	ConfidenceThreshold      float64                                   `json:"confidence_threshold" toml:"confidence_threshold"`
	PauseThreshold           float64                                   `json:"pause_threshold" toml:"pause_threshold"`
	HighBlastRadiusFileCount int                                       `json:"high_blast_radius_file_count" toml:"high_blast_radius_file_count"`
	HardGates                effectiveGuidedWorkflowsPolicyGatesConfig `json:"hard_gates" toml:"hard_gates"`
	ConditionalGates         effectiveGuidedWorkflowsPolicyGatesConfig `json:"conditional_gates" toml:"conditional_gates"`
}

type effectiveGuidedWorkflowsPolicyGatesConfig struct {
	AmbiguityBlocker         bool `json:"ambiguity_blocker" toml:"ambiguity_blocker"`
	ConfidenceBelowThreshold bool `json:"confidence_below_threshold" toml:"confidence_below_threshold"`
	HighBlastRadius          bool `json:"high_blast_radius" toml:"high_blast_radius"`
	SensitiveFiles           bool `json:"sensitive_files" toml:"sensitive_files"`
	PreCommitApproval        bool `json:"pre_commit_approval" toml:"pre_commit_approval"`
	FailingChecks            bool `json:"failing_checks" toml:"failing_checks"`
}

type effectiveGuidedWorkflowsRolloutConfig struct {
	TelemetryEnabled      bool `json:"telemetry_enabled" toml:"telemetry_enabled"`
	MaxActiveRuns         int  `json:"max_active_runs" toml:"max_active_runs"`
	AutomationEnabled     bool `json:"automation_enabled" toml:"automation_enabled"`
	AllowQualityChecks    bool `json:"allow_quality_checks" toml:"allow_quality_checks"`
	AllowCommit           bool `json:"allow_commit" toml:"allow_commit"`
	RequireCommitApproval bool `json:"require_commit_approval" toml:"require_commit_approval"`
	MaxRetryAttempts      int  `json:"max_retry_attempts" toml:"max_retry_attempts"`
}

type effectiveProvidersConfig struct {
	Codex    effectiveCodexProviderConfig   `json:"codex" toml:"codex"`
	Claude   effectiveClaudeProviderConfig  `json:"claude" toml:"claude"`
	OpenCode effectiveServerProviderConfig  `json:"opencode" toml:"opencode"`
	KiloCode effectiveServerProviderConfig  `json:"kilocode" toml:"kilocode"`
	Gemini   effectiveCommandProviderConfig `json:"gemini" toml:"gemini"`
}

type effectiveCommandProviderConfig struct {
	Command string `json:"command,omitempty" toml:"command,omitempty"`
}

type effectiveServerProviderConfig struct {
	Command        string `json:"command,omitempty" toml:"command,omitempty"`
	BaseURL        string `json:"base_url,omitempty" toml:"base_url,omitempty"`
	TokenEnv       string `json:"token_env,omitempty" toml:"token_env,omitempty"`
	TokenSet       bool   `json:"token_set,omitempty" toml:"token_set,omitempty"`
	Username       string `json:"username,omitempty" toml:"username,omitempty"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty" toml:"timeout_seconds,omitempty"`
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
	fs.Var(&scopes, "scope", "scope to print: core|ui|keybindings|workflow_templates|all (repeatable)")
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
	includeWorkflows := scopeSelected(scopes, configScopeWorkflows)

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
			out.Chat = &uiChatConfigOutput{
				TimestampMode: uiCfg.ChatTimestampMode(),
			}
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
		out.Notifications = &effectiveNotificationsConfig{
			Enabled:              coreCfg.NotificationsEnabled(),
			Triggers:             coreCfg.NotificationTriggers(),
			Methods:              coreCfg.NotificationMethods(),
			ScriptCommands:       coreCfg.NotificationScriptCommands(),
			ScriptTimeoutSeconds: coreCfg.NotificationScriptTimeoutSeconds(),
			DedupeWindowSeconds:  coreCfg.NotificationDedupeWindowSeconds(),
		}
		out.GuidedWorkflows = &effectiveGuidedWorkflowsConfig{
			Enabled:         coreCfg.GuidedWorkflowsEnabled(),
			AutoStart:       coreCfg.GuidedWorkflowsAutoStart(),
			CheckpointStyle: coreCfg.GuidedWorkflowsCheckpointStyle(),
			Mode:            coreCfg.GuidedWorkflowsMode(),
			Defaults: effectiveGuidedWorkflowsDefaultsConfig{
				Provider:           coreCfg.GuidedWorkflowsDefaultProvider(),
				Model:              coreCfg.GuidedWorkflowsDefaultModel(),
				Access:             string(coreCfg.GuidedWorkflowsDefaultAccessLevel()),
				Reasoning:          string(coreCfg.GuidedWorkflowsDefaultReasoningLevel()),
				ResolutionBoundary: coreCfg.GuidedWorkflowsDefaultResolutionBoundary(),
			},
			Policy: effectiveGuidedWorkflowsPolicyConfig{
				ConfidenceThreshold:      coreCfg.GuidedWorkflowsPolicyConfidenceThreshold(),
				PauseThreshold:           coreCfg.GuidedWorkflowsPolicyPauseThreshold(),
				HighBlastRadiusFileCount: coreCfg.GuidedWorkflowsPolicyHighBlastRadiusFileCount(),
				HardGates: effectiveGuidedWorkflowsPolicyGatesConfig{
					AmbiguityBlocker:         coreCfg.GuidedWorkflowsPolicyHardGateAmbiguityBlocker(),
					ConfidenceBelowThreshold: coreCfg.GuidedWorkflowsPolicyHardGateConfidenceBelowThreshold(),
					HighBlastRadius:          coreCfg.GuidedWorkflowsPolicyHardGateHighBlastRadius(),
					SensitiveFiles:           coreCfg.GuidedWorkflowsPolicyHardGateSensitiveFiles(),
					PreCommitApproval:        coreCfg.GuidedWorkflowsPolicyHardGatePreCommitApproval(),
					FailingChecks:            coreCfg.GuidedWorkflowsPolicyHardGateFailingChecks(),
				},
				ConditionalGates: effectiveGuidedWorkflowsPolicyGatesConfig{
					AmbiguityBlocker:         coreCfg.GuidedWorkflowsPolicyConditionalGateAmbiguityBlocker(),
					ConfidenceBelowThreshold: coreCfg.GuidedWorkflowsPolicyConditionalGateConfidenceBelowThreshold(),
					HighBlastRadius:          coreCfg.GuidedWorkflowsPolicyConditionalGateHighBlastRadius(),
					SensitiveFiles:           coreCfg.GuidedWorkflowsPolicyConditionalGateSensitiveFiles(),
					PreCommitApproval:        coreCfg.GuidedWorkflowsPolicyConditionalGatePreCommitApproval(),
					FailingChecks:            coreCfg.GuidedWorkflowsPolicyConditionalGateFailingChecks(),
				},
			},
			Rollout: effectiveGuidedWorkflowsRolloutConfig{
				TelemetryEnabled:      coreCfg.GuidedWorkflowsRolloutTelemetryEnabled(),
				MaxActiveRuns:         coreCfg.GuidedWorkflowsRolloutMaxActiveRuns(),
				AutomationEnabled:     coreCfg.GuidedWorkflowsRolloutAutomationEnabled(),
				AllowQualityChecks:    coreCfg.GuidedWorkflowsRolloutAllowQualityChecks(),
				AllowCommit:           coreCfg.GuidedWorkflowsRolloutAllowCommit(),
				RequireCommitApproval: coreCfg.GuidedWorkflowsRolloutRequireCommitApproval(),
				MaxRetryAttempts:      coreCfg.GuidedWorkflowsRolloutMaxRetryAttempts(),
			},
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
			OpenCode: effectiveServerProviderConfig{
				Command:        coreCfg.ProviderCommand("opencode"),
				BaseURL:        coreCfg.OpenCodeBaseURL("opencode"),
				TokenEnv:       coreCfg.OpenCodeTokenEnv("opencode"),
				TokenSet:       strings.TrimSpace(coreCfg.OpenCodeToken("opencode")) != "",
				Username:       coreCfg.OpenCodeUsername("opencode"),
				TimeoutSeconds: coreCfg.OpenCodeTimeoutSeconds("opencode"),
			},
			KiloCode: effectiveServerProviderConfig{
				Command:        coreCfg.ProviderCommand("kilocode"),
				BaseURL:        coreCfg.OpenCodeBaseURL("kilocode"),
				TokenEnv:       coreCfg.OpenCodeTokenEnv("kilocode"),
				TokenSet:       strings.TrimSpace(coreCfg.OpenCodeToken("kilocode")) != "",
				Username:       coreCfg.OpenCodeUsername("kilocode"),
				TimeoutSeconds: coreCfg.OpenCodeTimeoutSeconds("kilocode"),
			},
			Gemini: effectiveCommandProviderConfig{
				Command: coreCfg.ProviderCommand("gemini"),
			},
		}
	}

	if includeWorkflows {
		workflowTemplatesPath, err := config.WorkflowTemplatesPath()
		if err != nil {
			return configOutput{}, err
		}
		templates := []guidedworkflows.WorkflowTemplate{}
		if defaults {
			templates = guidedworkflows.DefaultWorkflowTemplates()
		} else {
			templates, err = store.NewFileWorkflowTemplateStore(workflowTemplatesPath).ListWorkflowTemplates(context.Background())
			if err != nil {
				return configOutput{}, err
			}
		}
		out.WorkflowTemplatesPath = workflowTemplatesPath
		out.WorkflowTemplates = &workflowTemplatesConfigOutput{
			Templates: templates,
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
	if scopeSelected(scopes, configScopeWorkflows) {
		if payload.WorkflowTemplates == nil {
			return workflowTemplatesConfigOutput{
				Templates: []guidedworkflows.WorkflowTemplate{},
			}
		}
		return *payload.WorkflowTemplates
	}
	if scopeSelected(scopes, configScopeUI) {
		chat := uiChatConfigOutput{TimestampMode: "relative"}
		if payload.Chat != nil {
			chat = *payload.Chat
		}
		return uiConfigOutput{
			Keybindings: uiKeybindingsConfigOutput{
				Path: payload.KeybindingsPath,
			},
			Chat: chat,
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
			Notifications: effectiveNotificationsConfig{
				Enabled:              true,
				Triggers:             []string{"turn.completed", "session.failed", "session.killed", "session.exited"},
				Methods:              []string{"auto"},
				ScriptTimeoutSeconds: 10,
				DedupeWindowSeconds:  5,
			},
			GuidedWorkflows: effectiveGuidedWorkflowsConfig{
				Enabled:         false,
				AutoStart:       false,
				CheckpointStyle: "confidence_weighted",
				Mode:            "guarded_autopilot",
				Policy: effectiveGuidedWorkflowsPolicyConfig{
					ConfidenceThreshold:      0.70,
					PauseThreshold:           0.60,
					HighBlastRadiusFileCount: 20,
					HardGates: effectiveGuidedWorkflowsPolicyGatesConfig{
						AmbiguityBlocker:         true,
						ConfidenceBelowThreshold: false,
						HighBlastRadius:          false,
						SensitiveFiles:           true,
						PreCommitApproval:        false,
						FailingChecks:            true,
					},
					ConditionalGates: effectiveGuidedWorkflowsPolicyGatesConfig{
						AmbiguityBlocker:         true,
						ConfidenceBelowThreshold: true,
						HighBlastRadius:          true,
						SensitiveFiles:           false,
						PreCommitApproval:        false,
						FailingChecks:            true,
					},
				},
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
		if payload.Notifications != nil {
			out.Notifications = *payload.Notifications
		}
		if payload.GuidedWorkflows != nil {
			out.GuidedWorkflows = *payload.GuidedWorkflows
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
			configScopeWorkflows:   {},
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
					configScopeWorkflows:   {},
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
	case configScopeWorkflows, "workflows", "workflow-templates":
		return configScopeWorkflows, nil
	default:
		return "", errors.New("invalid scope: must be core, ui, keybindings, workflow_templates, or all")
	}
}

func scopeSelected(scopes map[string]struct{}, scope string) bool {
	_, ok := scopes[scope]
	return ok
}
