package daemon

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
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

func (p *execProvider) Start(cfg StartSessionConfig, sink ProviderSink, items ProviderItemSink) (*providerProcess, error) {
	args := append([]string{}, cfg.Args...)
	if strings.EqualFold(strings.TrimSpace(p.providerName), "gemini") {
		additionalDirArgs, err := providerAdditionalDirectoryArgs(p.providerName, cfg.AdditionalDirectories)
		if err != nil {
			return nil, err
		}
		args = append(additionalDirArgs, args...)
	}
	cmd := exec.Command(p.cmdName, args...)
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

	var copyWG sync.WaitGroup
	copyWG.Add(2)
	go func() {
		defer copyWG.Done()
		_, _ = io.Copy(sink.StdoutWriter(), stdoutPipe)
	}()
	go func() {
		defer copyWG.Done()
		_, _ = io.Copy(sink.StderrWriter(), stderrPipe)
	}()

	waitFn := func() error {
		err := cmd.Wait()
		copyWG.Wait()
		return err
	}

	return &providerProcess{
		Process: cmd.Process,
		Wait:    waitFn,
	}, nil
}

func lookupCommand(cmdName string) (string, error) {
	if _, err := exec.LookPath(cmdName); err != nil {
		return "", fmt.Errorf("command not found: %s", cmdName)
	}
	return cmdName, nil
}
