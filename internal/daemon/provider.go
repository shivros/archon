package daemon

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"control/internal/config"
	"control/internal/providers"
)

type Provider interface {
	Name() string
	Command() string
	Start(cfg StartSessionConfig, sink ProviderLogSink, items ProviderItemSink) (*providerProcess, error)
}

type ProviderLogSink interface {
	StdoutWriter() io.Writer
	StderrWriter() io.Writer
	Write(stream string, data []byte)
}

type ProviderItemSink interface {
	Append(item map[string]any)
}

type providerProcess struct {
	Process   *os.Process
	Wait      func() error
	Interrupt func() error
	ThreadID  string
	Send      func([]byte) error
}

type providerFactory func(def providers.Definition, commandName string) (Provider, error)

var providerFactories = map[providers.Runtime]providerFactory{
	providers.RuntimeCodex: func(_ providers.Definition, commandName string) (Provider, error) {
		return newCodexProvider(commandName)
	},
	providers.RuntimeClaude: func(_ providers.Definition, commandName string) (Provider, error) {
		return newClaudeProvider(commandName)
	},
	providers.RuntimeExec: func(def providers.Definition, commandName string) (Provider, error) {
		return newExecProvider(def.Name, commandName, nil)
	},
	providers.RuntimeOpenCodeServer: func(def providers.Definition, _ string) (Provider, error) {
		return newOpenCodeProvider(def.Name)
	},
	providers.RuntimeCustom: func(def providers.Definition, commandName string) (Provider, error) {
		return newExecProvider(def.Name, commandName, nil)
	},
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
	cmdName := ""
	if runtimeRequiresCommand(def.Runtime) {
		var err error
		cmdName, err = resolveProviderCommandName(def, customCmd)
		if err != nil {
			return nil, err
		}
	}
	factory, ok := providerFactories[def.Runtime]
	if !ok || factory == nil {
		return nil, fmt.Errorf("provider runtime is not supported: %s", def.Runtime)
	}
	return factory(def, cmdName)
}

func runtimeRequiresCommand(runtime providers.Runtime) bool {
	switch runtime {
	case providers.RuntimeOpenCodeServer:
		return false
	default:
		return true
	}
}

func resolveProviderCommandName(def providers.Definition, customCmd string) (string, error) {
	if def.Runtime == providers.RuntimeCustom {
		if strings.TrimSpace(customCmd) == "" {
			return "", errors.New("custom provider requires cmd")
		}
		return lookupCommand(customCmd)
	}
	coreCfg, err := config.LoadCoreConfig()
	if err != nil {
		return "", err
	}
	if override := strings.TrimSpace(coreCfg.ProviderCommand(def.Name)); override != "" {
		return lookupCommand(override)
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
