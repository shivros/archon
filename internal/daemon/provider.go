package daemon

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

type Provider interface {
	Name() string
	Command() string
	Start(cfg StartSessionConfig, sink *logSink, items *itemSink) (*providerProcess, error)
}

type providerProcess struct {
	Process   *os.Process
	Wait      func() error
	Interrupt func() error
	ThreadID  string
	Send      func([]byte) error
}

func ResolveProvider(provider, customCmd string) (Provider, error) {
	if provider == "" {
		return nil, errors.New("provider is required")
	}

	switch normalizeProvider(provider) {
	case "codex":
		return newCodexProvider()
	case "claude":
		return newClaudeProvider()
	case "opencode":
		cmd, err := findOpenCodeCommand()
		return newExecProvider("opencode", cmd, err)
	case "gemini":
		cmd, err := findCommand("ARCHON_GEMINI_CMD", "gemini")
		return newExecProvider("gemini", cmd, err)
	case "custom":
		if customCmd == "" {
			return nil, errors.New("custom provider requires cmd")
		}
		cmd, err := lookupCommand(customCmd)
		return newExecProvider("custom", cmd, err)
	default:
		return nil, fmt.Errorf("unknown provider: %s", provider)
	}
}

func normalizeProvider(provider string) string {
	return strings.ToLower(strings.TrimSpace(provider))
}
