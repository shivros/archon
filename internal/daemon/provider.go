package daemon

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"control/internal/providers"
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
	name := providers.Normalize(provider)
	if name == "" {
		return nil, errors.New("provider is required")
	}
	def, ok := providers.Lookup(name)
	if !ok {
		return nil, fmt.Errorf("unknown provider: %s", provider)
	}
	cmdName, err := resolveProviderCommandName(def, customCmd)
	if err != nil {
		return nil, err
	}

	switch def.Runtime {
	case providers.RuntimeCodex:
		return newCodexProvider(cmdName)
	case providers.RuntimeClaude:
		return newClaudeProvider(cmdName)
	case providers.RuntimeExec, providers.RuntimeCustom:
		return newExecProvider(def.Name, cmdName, nil)
	default:
		return nil, fmt.Errorf("provider runtime is not supported: %s", def.Runtime)
	}
}

func resolveProviderCommandName(def providers.Definition, customCmd string) (string, error) {
	if def.Runtime == providers.RuntimeCustom {
		if strings.TrimSpace(customCmd) == "" {
			return "", errors.New("custom provider requires cmd")
		}
		return lookupCommand(customCmd)
	}
	if cmd := strings.TrimSpace(def.CommandEnv); cmd != "" {
		if envVal := strings.TrimSpace(os.Getenv(cmd)); envVal != "" {
			return lookupCommand(envVal)
		}
	}
	candidates := def.CommandCandidates
	if len(candidates) == 0 {
		return "", errors.New("provider command is not configured")
	}
	if len(candidates) == 1 {
		return lookupCommand(candidates[0])
	}
	var valid []string
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		valid = append(valid, candidate)
		if cmd, err := lookupCommand(candidate); err == nil {
			return cmd, nil
		}
	}
	if len(valid) == 0 {
		return "", errors.New("provider command is not configured")
	}
	return "", fmt.Errorf("command not found: %s", strings.Join(valid, " or "))
}
