package daemon

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

type execProvider struct {
	providerName string
	cmdName      string
}

func newExecProvider(providerName string, cmdName string, err error) (Provider, error) {
	if err != nil {
		return nil, err
	}
	if cmdName == "" {
		return nil, errors.New("command name is required")
	}
	return &execProvider{providerName: providerName, cmdName: cmdName}, nil
}

func (p *execProvider) Name() string {
	return p.providerName
}

func (p *execProvider) Command() string {
	return p.cmdName
}

func (p *execProvider) Start(cfg StartSessionConfig, sink *logSink, items *itemSink) (*providerProcess, error) {
	cmd := exec.Command(p.cmdName, cfg.Args...)
	if cfg.Cwd != "" {
		cmd.Dir = cfg.Cwd
	}
	if len(cfg.Env) > 0 {
		cmd.Env = append(os.Environ(), cfg.Env...)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	go func() {
		_, _ = io.Copy(sink.StdoutWriter(), stdoutPipe)
	}()
	go func() {
		_, _ = io.Copy(sink.StderrWriter(), stderrPipe)
	}()

	return &providerProcess{
		Process: cmd.Process,
		Wait:    cmd.Wait,
	}, nil
}

func findCommand(envKey, fallback string) (string, error) {
	if override := strings.TrimSpace(os.Getenv(envKey)); override != "" {
		return lookupCommand(override)
	}
	return lookupCommand(fallback)
}

func lookupCommand(cmdName string) (string, error) {
	if _, err := exec.LookPath(cmdName); err != nil {
		return "", fmt.Errorf("command not found: %s", cmdName)
	}
	return cmdName, nil
}

func findOpenCodeCommand() (string, error) {
	if override := strings.TrimSpace(os.Getenv("ARCHON_OPENCODE_CMD")); override != "" {
		return lookupCommand(override)
	}
	if _, err := exec.LookPath("opencode"); err == nil {
		return "opencode", nil
	}
	if _, err := exec.LookPath("opencode-cli"); err == nil {
		return "opencode-cli", nil
	}
	return "", errors.New("command not found: opencode or opencode-cli")
}
