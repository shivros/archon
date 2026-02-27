package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"control/internal/guidedworkflows"
	"control/internal/providers"
	"control/internal/types"

	toml "github.com/pelletier/go-toml/v2"
)

const defaultDaemonAddress = "127.0.0.1:7777"
const (
	defaultCodexModel                              = "gpt-5.1-codex"
	defaultClaudeModel                             = "sonnet"
	defaultGuidedWorkflowsCheckpointStyle          = "confidence_weighted"
	defaultGuidedWorkflowsMode                     = "guarded_autopilot"
	defaultGuidedWorkflowsConfidenceThreshold      = 0.70
	defaultGuidedWorkflowsPauseThreshold           = 0.60
	defaultGuidedWorkflowsHighBlastRadiusFileCount = 20
	defaultGuidedWorkflowsRolloutTelemetryEnabled  = true
	defaultGuidedWorkflowsRolloutMaxActiveRuns     = 3
	defaultGuidedWorkflowsRolloutAutomationEnabled = false
	defaultGuidedWorkflowsRolloutAllowQuality      = false
	defaultGuidedWorkflowsRolloutAllowCommit       = false
	defaultGuidedWorkflowsRolloutCommitApproval    = true
	defaultGuidedWorkflowsRolloutMaxRetryAttempts  = 2
	maxGuidedWorkflowsRolloutRetryAttempts         = 5
)

var defaultCodexModels = []string{
	"gpt-5.1-codex",
	"gpt-5.2-codex",
	"gpt-5.3-codex",
	"gpt-5.1-codex-max",
}

var defaultClaudeModels = []string{"sonnet", "opus"}
var defaultNotificationTriggers = []string{
	"turn.completed",
	"session.failed",
	"session.killed",
	"session.exited",
}
var defaultNotificationMethods = []string{"auto"}

type CoreConfig struct {
	Daemon          CoreDaemonConfig          `toml:"daemon"`
	Providers       CoreProvidersConfig       `toml:"providers"`
	Logging         CoreLoggingConfig         `toml:"logging"`
	Debug           CoreDebugConfig           `toml:"debug"`
	Notifications   CoreNotificationsConfig   `toml:"notifications"`
	GuidedWorkflows CoreGuidedWorkflowsConfig `toml:"guided_workflows"`
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

type CoreNotificationsConfig struct {
	Enabled              *bool    `toml:"enabled"`
	Triggers             []string `toml:"triggers"`
	Methods              []string `toml:"methods"`
	ScriptCommands       []string `toml:"script_commands"`
	ScriptTimeoutSeconds int      `toml:"script_timeout_seconds"`
	DedupeWindowSeconds  int      `toml:"dedupe_window_seconds"`
}

type CoreGuidedWorkflowsConfig struct {
	Enabled         *bool                             `toml:"enabled"`
	AutoStart       *bool                             `toml:"auto_start"`
	CheckpointStyle string                            `toml:"checkpoint_style"`
	Mode            string                            `toml:"mode"`
	Defaults        CoreGuidedWorkflowsDefaultsConfig `toml:"defaults"`
	Policy          CoreGuidedWorkflowsPolicyConfig   `toml:"policy"`
	Rollout         CoreGuidedWorkflowsRolloutConfig  `toml:"rollout"`
}

type CoreGuidedWorkflowsDefaultsConfig struct {
	Provider           string `toml:"provider"`
	Model              string `toml:"model"`
	Access             string `toml:"access"`
	Reasoning          string `toml:"reasoning"`
	Risk               string `toml:"risk"` // deprecated; ignored
	ResolutionBoundary string `toml:"resolution_boundary"`
}

type CoreGuidedWorkflowsPolicyConfig struct {
	ConfidenceThreshold      float64                              `toml:"confidence_threshold"`
	PauseThreshold           float64                              `toml:"pause_threshold"`
	HighBlastRadiusFileCount int                                  `toml:"high_blast_radius_file_count"`
	HardGates                CoreGuidedWorkflowsPolicyGatesConfig `toml:"hard_gates"`
	ConditionalGates         CoreGuidedWorkflowsPolicyGatesConfig `toml:"conditional_gates"`
}

type CoreGuidedWorkflowsPolicyGatesConfig struct {
	AmbiguityBlocker         *bool `toml:"ambiguity_blocker"`
	ConfidenceBelowThreshold *bool `toml:"confidence_below_threshold"`
	HighBlastRadius          *bool `toml:"high_blast_radius"`
	SensitiveFiles           *bool `toml:"sensitive_files"`
	PreCommitApproval        *bool `toml:"pre_commit_approval"`
	FailingChecks            *bool `toml:"failing_checks"`
}

type CoreGuidedWorkflowsRolloutConfig struct {
	TelemetryEnabled      *bool `toml:"telemetry_enabled"`
	MaxActiveRuns         int   `toml:"max_active_runs"`
	AutomationEnabled     *bool `toml:"automation_enabled"`
	AllowQualityChecks    *bool `toml:"allow_quality_checks"`
	AllowCommit           *bool `toml:"allow_commit"`
	RequireCommitApproval *bool `toml:"require_commit_approval"`
	MaxRetryAttempts      int   `toml:"max_retry_attempts"`
}

type CoreProvidersConfig struct {
	Codex    CoreCodexProviderConfig    `toml:"codex"`
	Claude   CoreClaudeProviderConfig   `toml:"claude"`
	OpenCode CoreOpenCodeProviderConfig `toml:"opencode"`
	KiloCode CoreOpenCodeProviderConfig `toml:"kilocode"`
	Gemini   CoreCommandProviderConfig  `toml:"gemini"`
}

type CoreCommandProviderConfig struct {
	Command string `toml:"command"`
}

type CoreOpenCodeProviderConfig struct {
	Command        string `toml:"command"`
	BaseURL        string `toml:"base_url"`
	Token          string `toml:"token"`
	TokenEnv       string `toml:"token_env"`
	Username       string `toml:"username"`
	TimeoutSeconds int    `toml:"timeout_seconds"`
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
	Chat        UIChatConfig        `toml:"chat"`
	Sidebar     UISidebarConfig     `toml:"sidebar"`
}

type UIKeybindingsConfig struct {
	Path string `toml:"path"`
}

type UIInputConfig struct {
	MultilineMinHeight int `toml:"multiline_min_height"`
	MultilineMaxHeight int `toml:"multiline_max_height"`
}

type UIChatConfig struct {
	TimestampMode string `toml:"timestamp_mode"`
}

type UISidebarConfig struct {
	ExpandByDefault *bool `toml:"expand_by_default"`
	ShowRecents     *bool `toml:"show_recents"`
}

func DefaultCoreConfig() CoreConfig {
	return CoreConfig{
		Daemon: CoreDaemonConfig{
			Address: defaultDaemonAddress,
		},
		Logging: CoreLoggingConfig{
			Level: "info",
		},
		Notifications: CoreNotificationsConfig{
			Enabled:              boolPtr(true),
			Triggers:             append([]string{}, defaultNotificationTriggers...),
			Methods:              append([]string{}, defaultNotificationMethods...),
			ScriptTimeoutSeconds: 10,
			DedupeWindowSeconds:  5,
		},
		GuidedWorkflows: CoreGuidedWorkflowsConfig{
			Enabled:         boolPtr(false),
			AutoStart:       boolPtr(false),
			CheckpointStyle: defaultGuidedWorkflowsCheckpointStyle,
			Mode:            defaultGuidedWorkflowsMode,
			Policy: CoreGuidedWorkflowsPolicyConfig{
				ConfidenceThreshold:      defaultGuidedWorkflowsConfidenceThreshold,
				PauseThreshold:           defaultGuidedWorkflowsPauseThreshold,
				HighBlastRadiusFileCount: defaultGuidedWorkflowsHighBlastRadiusFileCount,
				HardGates: CoreGuidedWorkflowsPolicyGatesConfig{
					AmbiguityBlocker:  boolPtr(true),
					SensitiveFiles:    boolPtr(true),
					PreCommitApproval: boolPtr(false),
					FailingChecks:     boolPtr(true),
				},
				ConditionalGates: CoreGuidedWorkflowsPolicyGatesConfig{
					AmbiguityBlocker:         boolPtr(true),
					ConfidenceBelowThreshold: boolPtr(true),
					HighBlastRadius:          boolPtr(true),
					SensitiveFiles:           boolPtr(false),
					PreCommitApproval:        boolPtr(false),
					FailingChecks:            boolPtr(true),
				},
			},
			Rollout: CoreGuidedWorkflowsRolloutConfig{
				TelemetryEnabled:      boolPtr(defaultGuidedWorkflowsRolloutTelemetryEnabled),
				MaxActiveRuns:         defaultGuidedWorkflowsRolloutMaxActiveRuns,
				AutomationEnabled:     boolPtr(defaultGuidedWorkflowsRolloutAutomationEnabled),
				AllowQualityChecks:    boolPtr(defaultGuidedWorkflowsRolloutAllowQuality),
				AllowCommit:           boolPtr(defaultGuidedWorkflowsRolloutAllowCommit),
				RequireCommitApproval: boolPtr(defaultGuidedWorkflowsRolloutCommitApproval),
				MaxRetryAttempts:      defaultGuidedWorkflowsRolloutMaxRetryAttempts,
			},
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

func (c CoreConfig) NotificationsEnabled() bool {
	if c.Notifications.Enabled == nil {
		return true
	}
	return *c.Notifications.Enabled
}

func (c CoreConfig) NotificationTriggers() []string {
	values := normalizedList(c.Notifications.Triggers)
	if len(values) == 0 {
		values = append([]string{}, defaultNotificationTriggers...)
	}
	return values
}

func (c CoreConfig) NotificationMethods() []string {
	values := normalizedList(c.Notifications.Methods)
	if len(values) == 0 {
		values = append([]string{}, defaultNotificationMethods...)
	}
	return values
}

func (c CoreConfig) NotificationScriptCommands() []string {
	return normalizedList(c.Notifications.ScriptCommands)
}

func (c CoreConfig) NotificationScriptTimeoutSeconds() int {
	if c.Notifications.ScriptTimeoutSeconds > 0 {
		return c.Notifications.ScriptTimeoutSeconds
	}
	return 10
}

func (c CoreConfig) NotificationDedupeWindowSeconds() int {
	if c.Notifications.DedupeWindowSeconds > 0 {
		return c.Notifications.DedupeWindowSeconds
	}
	return 5
}

func (c CoreConfig) GuidedWorkflowsEnabled() bool {
	if c.GuidedWorkflows.Enabled == nil {
		return false
	}
	return *c.GuidedWorkflows.Enabled
}

func (c CoreConfig) GuidedWorkflowsAutoStart() bool {
	if c.GuidedWorkflows.AutoStart == nil {
		return false
	}
	return *c.GuidedWorkflows.AutoStart
}

func (c CoreConfig) GuidedWorkflowsCheckpointStyle() string {
	switch normalizeGuidedWorkflowsValue(c.GuidedWorkflows.CheckpointStyle) {
	case "confidence_weighted":
		return "confidence_weighted"
	default:
		return defaultGuidedWorkflowsCheckpointStyle
	}
}

func (c CoreConfig) GuidedWorkflowsMode() string {
	switch normalizeGuidedWorkflowsValue(c.GuidedWorkflows.Mode) {
	case "guarded_autopilot":
		return "guarded_autopilot"
	default:
		return defaultGuidedWorkflowsMode
	}
}

func (c CoreConfig) GuidedWorkflowsDefaultProvider() string {
	provider := providers.Normalize(c.GuidedWorkflows.Defaults.Provider)
	if provider == "" {
		return ""
	}
	if _, ok := providers.Lookup(provider); !ok {
		return ""
	}
	return provider
}

func (c CoreConfig) GuidedWorkflowsDefaultModel() string {
	return strings.TrimSpace(c.GuidedWorkflows.Defaults.Model)
}

func (c CoreConfig) GuidedWorkflowsDefaultAccessLevel() types.AccessLevel {
	level, ok := normalizeGuidedWorkflowsAccessLevel(c.GuidedWorkflows.Defaults.Access)
	if !ok {
		return ""
	}
	return level
}

func (c CoreConfig) GuidedWorkflowsDefaultReasoningLevel() types.ReasoningLevel {
	level, ok := normalizeGuidedWorkflowsReasoningLevel(c.GuidedWorkflows.Defaults.Reasoning)
	if !ok {
		return ""
	}
	return level
}

func (c CoreConfig) GuidedWorkflowsDefaultResolutionBoundary() string {
	level, ok := normalizeGuidedWorkflowsSensitivityPreset(c.GuidedWorkflows.Defaults.ResolutionBoundary)
	if !ok {
		return ""
	}
	return level
}

func (c CoreConfig) GuidedWorkflowsPolicyConfidenceThreshold() float64 {
	value := c.GuidedWorkflows.Policy.ConfidenceThreshold
	if value <= 0 || value > 1 {
		return defaultGuidedWorkflowsConfidenceThreshold
	}
	return value
}

func (c CoreConfig) GuidedWorkflowsPolicyPauseThreshold() float64 {
	value := c.GuidedWorkflows.Policy.PauseThreshold
	if value <= 0 || value > 1 {
		return defaultGuidedWorkflowsPauseThreshold
	}
	return value
}

func (c CoreConfig) GuidedWorkflowsPolicyHighBlastRadiusFileCount() int {
	value := c.GuidedWorkflows.Policy.HighBlastRadiusFileCount
	if value <= 0 {
		return defaultGuidedWorkflowsHighBlastRadiusFileCount
	}
	return value
}

func (c CoreConfig) GuidedWorkflowsPolicyHardGateAmbiguityBlocker() bool {
	return boolFromPtrWithDefault(c.GuidedWorkflows.Policy.HardGates.AmbiguityBlocker, true)
}

func (c CoreConfig) GuidedWorkflowsPolicyHardGateConfidenceBelowThreshold() bool {
	return boolFromPtrWithDefault(c.GuidedWorkflows.Policy.HardGates.ConfidenceBelowThreshold, false)
}

func (c CoreConfig) GuidedWorkflowsPolicyHardGateHighBlastRadius() bool {
	return boolFromPtrWithDefault(c.GuidedWorkflows.Policy.HardGates.HighBlastRadius, false)
}

func (c CoreConfig) GuidedWorkflowsPolicyHardGateSensitiveFiles() bool {
	return boolFromPtrWithDefault(c.GuidedWorkflows.Policy.HardGates.SensitiveFiles, true)
}

func (c CoreConfig) GuidedWorkflowsPolicyHardGatePreCommitApproval() bool {
	return boolFromPtrWithDefault(c.GuidedWorkflows.Policy.HardGates.PreCommitApproval, false)
}

func (c CoreConfig) GuidedWorkflowsPolicyHardGateFailingChecks() bool {
	return boolFromPtrWithDefault(c.GuidedWorkflows.Policy.HardGates.FailingChecks, true)
}

func (c CoreConfig) GuidedWorkflowsPolicyConditionalGateAmbiguityBlocker() bool {
	return boolFromPtrWithDefault(c.GuidedWorkflows.Policy.ConditionalGates.AmbiguityBlocker, true)
}

func (c CoreConfig) GuidedWorkflowsPolicyConditionalGateConfidenceBelowThreshold() bool {
	return boolFromPtrWithDefault(c.GuidedWorkflows.Policy.ConditionalGates.ConfidenceBelowThreshold, true)
}

func (c CoreConfig) GuidedWorkflowsPolicyConditionalGateHighBlastRadius() bool {
	return boolFromPtrWithDefault(c.GuidedWorkflows.Policy.ConditionalGates.HighBlastRadius, true)
}

func (c CoreConfig) GuidedWorkflowsPolicyConditionalGateSensitiveFiles() bool {
	return boolFromPtrWithDefault(c.GuidedWorkflows.Policy.ConditionalGates.SensitiveFiles, false)
}

func (c CoreConfig) GuidedWorkflowsPolicyConditionalGatePreCommitApproval() bool {
	return boolFromPtrWithDefault(c.GuidedWorkflows.Policy.ConditionalGates.PreCommitApproval, false)
}

func (c CoreConfig) GuidedWorkflowsPolicyConditionalGateFailingChecks() bool {
	return boolFromPtrWithDefault(c.GuidedWorkflows.Policy.ConditionalGates.FailingChecks, true)
}

func (c CoreConfig) GuidedWorkflowsRolloutTelemetryEnabled() bool {
	return boolFromPtrWithDefault(c.GuidedWorkflows.Rollout.TelemetryEnabled, defaultGuidedWorkflowsRolloutTelemetryEnabled)
}

func (c CoreConfig) GuidedWorkflowsRolloutMaxActiveRuns() int {
	value := c.GuidedWorkflows.Rollout.MaxActiveRuns
	if value <= 0 {
		return defaultGuidedWorkflowsRolloutMaxActiveRuns
	}
	return value
}

func (c CoreConfig) GuidedWorkflowsRolloutAutomationEnabled() bool {
	return boolFromPtrWithDefault(c.GuidedWorkflows.Rollout.AutomationEnabled, defaultGuidedWorkflowsRolloutAutomationEnabled)
}

func (c CoreConfig) GuidedWorkflowsRolloutAllowQualityChecks() bool {
	return boolFromPtrWithDefault(c.GuidedWorkflows.Rollout.AllowQualityChecks, defaultGuidedWorkflowsRolloutAllowQuality)
}

func (c CoreConfig) GuidedWorkflowsRolloutAllowCommit() bool {
	return boolFromPtrWithDefault(c.GuidedWorkflows.Rollout.AllowCommit, defaultGuidedWorkflowsRolloutAllowCommit)
}

func (c CoreConfig) GuidedWorkflowsRolloutRequireCommitApproval() bool {
	return boolFromPtrWithDefault(c.GuidedWorkflows.Rollout.RequireCommitApproval, defaultGuidedWorkflowsRolloutCommitApproval)
}

func (c CoreConfig) GuidedWorkflowsRolloutMaxRetryAttempts() int {
	value := c.GuidedWorkflows.Rollout.MaxRetryAttempts
	if value <= 0 {
		return defaultGuidedWorkflowsRolloutMaxRetryAttempts
	}
	if value > maxGuidedWorkflowsRolloutRetryAttempts {
		return maxGuidedWorkflowsRolloutRetryAttempts
	}
	return value
}

func (c CoreConfig) ProviderCommand(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "codex":
		return strings.TrimSpace(c.Providers.Codex.Command)
	case "claude":
		return strings.TrimSpace(c.Providers.Claude.Command)
	case "opencode":
		return strings.TrimSpace(c.Providers.OpenCode.Command)
	case "kilocode":
		return strings.TrimSpace(c.Providers.KiloCode.Command)
	case "gemini":
		return strings.TrimSpace(c.Providers.Gemini.Command)
	default:
		return ""
	}
}

func (c CoreConfig) OpenCodeBaseURL(provider string) string {
	cfg := c.openCodeProviderConfig(provider)
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL != "" {
		return baseURL
	}
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "kilocode":
		return "http://127.0.0.1:4097"
	default:
		return "http://127.0.0.1:4096"
	}
}

func (c CoreConfig) OpenCodeToken(provider string) string {
	cfg := c.openCodeProviderConfig(provider)
	return strings.TrimSpace(cfg.Token)
}

func (c CoreConfig) OpenCodeTokenEnv(provider string) string {
	cfg := c.openCodeProviderConfig(provider)
	return strings.TrimSpace(cfg.TokenEnv)
}

func (c CoreConfig) OpenCodeUsername(provider string) string {
	name := strings.ToLower(strings.TrimSpace(provider))
	cfg := c.openCodeProviderConfig(name)
	username := strings.TrimSpace(cfg.Username)
	if username != "" {
		return username
	}
	if name == "kilocode" {
		return "kilocode"
	}
	return "opencode"
}

func (c CoreConfig) OpenCodeTimeoutSeconds(provider string) int {
	cfg := c.openCodeProviderConfig(provider)
	if cfg.TimeoutSeconds > 0 {
		return cfg.TimeoutSeconds
	}
	return 180
}

func (c CoreConfig) openCodeProviderConfig(provider string) CoreOpenCodeProviderConfig {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "kilocode":
		return c.Providers.KiloCode
	default:
		return c.Providers.OpenCode
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
		Chat: UIChatConfig{
			TimestampMode: "relative",
		},
		Sidebar: UISidebarConfig{
			ExpandByDefault: boolPtr(true),
			ShowRecents:     boolPtr(true),
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

func (c UIConfig) ChatTimestampMode() string {
	mode := strings.ToLower(strings.TrimSpace(c.Chat.TimestampMode))
	switch mode {
	case "iso":
		return "iso"
	default:
		return "relative"
	}
}

func (c UIConfig) SidebarExpandByDefault() bool {
	if c.Sidebar.ExpandByDefault == nil {
		return true
	}
	return *c.Sidebar.ExpandByDefault
}

func (c UIConfig) SidebarShowRecents() bool {
	if c.Sidebar.ShowRecents == nil {
		return true
	}
	return *c.Sidebar.ShowRecents
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

func normalizeGuidedWorkflowsValue(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	value = strings.ReplaceAll(value, "-", "_")
	value = strings.ReplaceAll(value, " ", "_")
	return value
}

func normalizeGuidedWorkflowsAccessLevel(raw string) (types.AccessLevel, bool) {
	switch normalizeGuidedWorkflowsValue(raw) {
	case "":
		return "", true
	case "read_only", "readonly":
		return types.AccessReadOnly, true
	case "on_request", "onrequest":
		return types.AccessOnRequest, true
	case "full_access", "fullaccess":
		return types.AccessFull, true
	default:
		return "", false
	}
}

func normalizeGuidedWorkflowsReasoningLevel(raw string) (types.ReasoningLevel, bool) {
	switch normalizeGuidedWorkflowsValue(raw) {
	case "":
		return "", true
	case "low":
		return types.ReasoningLow, true
	case "medium":
		return types.ReasoningMedium, true
	case "high":
		return types.ReasoningHigh, true
	case "extra_high", "extrahigh":
		return types.ReasoningExtraHigh, true
	default:
		return "", false
	}
}

func normalizeGuidedWorkflowsSensitivityPreset(raw string) (string, bool) {
	preset, ok := guidedworkflows.NormalizePolicyPreset(raw)
	if !ok {
		return "", false
	}
	return string(preset), true
}

func boolPtr(value bool) *bool {
	v := value
	return &v
}

func boolFromPtrWithDefault(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}
