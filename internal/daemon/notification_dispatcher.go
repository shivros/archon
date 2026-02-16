package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"control/internal/logging"
	"control/internal/types"
)

type NotificationScriptRunner interface {
	Run(ctx context.Context, command string, payload []byte, env []string) error
}

type shellNotificationScriptRunner struct{}

func (shellNotificationScriptRunner) Run(ctx context.Context, command string, payload []byte, env []string) error {
	stderr := &bytes.Buffer{}
	cmd := exec.CommandContext(ctx, "sh", "-lc", command)
	cmd.Stdin = bytes.NewReader(payload)
	cmd.Stdout = io.Discard
	cmd.Stderr = stderr
	cmd.Env = append(os.Environ(), env...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("script %q failed: %w (%s)", command, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

type defaultNotificationDispatcher struct {
	sinks        map[types.NotificationMethod]NotificationSink
	scriptRunner NotificationScriptRunner
	logger       logging.Logger
}

func NewNotificationDispatcher(sinks []NotificationSink, logger logging.Logger) NotificationDispatcher {
	if logger == nil {
		logger = logging.Nop()
	}
	byMethod := map[types.NotificationMethod]NotificationSink{}
	for _, sink := range sinks {
		if sink == nil {
			continue
		}
		byMethod[sink.Method()] = sink
	}
	return &defaultNotificationDispatcher{
		sinks:        byMethod,
		scriptRunner: shellNotificationScriptRunner{},
		logger:       logger,
	}
}

func (d *defaultNotificationDispatcher) Dispatch(ctx context.Context, event types.NotificationEvent, settings types.NotificationSettings) error {
	var dispatchErr error
	if len(settings.Methods) > 0 {
		delivered := false
		for _, method := range settings.Methods {
			switched, err := d.dispatchMethod(ctx, method, event, settings)
			if err == nil {
				delivered = delivered || switched
				continue
			}
			dispatchErr = errors.Join(dispatchErr, err)
		}
		if !delivered && len(settings.ScriptCommands) == 0 && dispatchErr != nil {
			return dispatchErr
		}
	}
	if err := d.dispatchScripts(ctx, event, settings); err != nil {
		dispatchErr = errors.Join(dispatchErr, err)
	}
	return dispatchErr
}

func (d *defaultNotificationDispatcher) dispatchMethod(ctx context.Context, method types.NotificationMethod, event types.NotificationEvent, settings types.NotificationSettings) (bool, error) {
	if method == types.NotificationMethodAuto {
		for _, fallback := range []types.NotificationMethod{types.NotificationMethodDunstify, types.NotificationMethodNotifySend, types.NotificationMethodBell} {
			sink, ok := d.sinks[fallback]
			if !ok || sink == nil {
				continue
			}
			if err := sink.Notify(ctx, event, settings); err == nil {
				return true, nil
			}
		}
		return false, errors.New("no notification sink available for auto")
	}
	sink, ok := d.sinks[method]
	if !ok || sink == nil {
		return false, fmt.Errorf("unknown notification method: %s", method)
	}
	if err := sink.Notify(ctx, event, settings); err != nil {
		return false, err
	}
	return true, nil
}

func (d *defaultNotificationDispatcher) dispatchScripts(ctx context.Context, event types.NotificationEvent, settings types.NotificationSettings) error {
	if len(settings.ScriptCommands) == 0 {
		return nil
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	runner := d.scriptRunner
	if runner == nil {
		runner = shellNotificationScriptRunner{}
	}
	var runErr error
	for _, command := range settings.ScriptCommands {
		command = strings.TrimSpace(command)
		if command == "" {
			continue
		}
		scriptCtx, cancel := context.WithTimeout(ctx, time.Duration(settings.ScriptTimeoutSeconds)*time.Second)
		err := runner.Run(scriptCtx, command, payload, notificationScriptEnv(event))
		cancel()
		if err != nil {
			runErr = errors.Join(runErr, err)
		}
	}
	return runErr
}

type notifySendSink struct{}

func (notifySendSink) Method() types.NotificationMethod {
	return types.NotificationMethodNotifySend
}

func (notifySendSink) Notify(ctx context.Context, event types.NotificationEvent, settings types.NotificationSettings) error {
	if _, err := exec.LookPath("notify-send"); err != nil {
		return err
	}
	title, body := notificationTitleBody(event)
	cmd := exec.CommandContext(ctx, "notify-send", title, body)
	return cmd.Run()
}

type dunstifySink struct{}

func (dunstifySink) Method() types.NotificationMethod {
	return types.NotificationMethodDunstify
}

func (dunstifySink) Notify(ctx context.Context, event types.NotificationEvent, settings types.NotificationSettings) error {
	if _, err := exec.LookPath("dunstify"); err != nil {
		return err
	}
	title, body := notificationTitleBody(event)
	cmd := exec.CommandContext(ctx, "dunstify", title, body)
	return cmd.Run()
}

type bellSink struct{}

func (bellSink) Method() types.NotificationMethod {
	return types.NotificationMethodBell
}

func (bellSink) Notify(ctx context.Context, event types.NotificationEvent, settings types.NotificationSettings) error {
	_, err := fmt.Fprint(os.Stdout, "\a")
	return err
}

func defaultNotificationSinks() []NotificationSink {
	return []NotificationSink{
		dunstifySink{},
		notifySendSink{},
		bellSink{},
	}
}
