package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	controlclient "control/internal/client"
	"control/internal/config"
	"control/internal/daemon"
	"control/internal/store"
)

type DaemonCommand struct {
	stderr     io.Writer
	runDaemon  func(background bool) error
	killDaemon func() error
}

func NewDaemonCommand(stderr io.Writer, runDaemon func(background bool) error, killDaemon func() error) *DaemonCommand {
	return &DaemonCommand{
		stderr:     stderr,
		runDaemon:  runDaemon,
		killDaemon: killDaemon,
	}
}

func (c *DaemonCommand) Run(args []string) error {
	fs := flag.NewFlagSet("daemon", flag.ContinueOnError)
	fs.SetOutput(c.stderr)
	background := fs.Bool("background", false, "run in background (logs to file)")
	kill := fs.Bool("kill", false, "stop any running daemon and exit")
	force := fs.Bool("force", false, "stop any running daemon before starting")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *kill {
		return c.killDaemon()
	}
	if *force {
		if err := c.killDaemon(); err != nil {
			return err
		}
	}
	return c.runDaemon(*background)
}

func runDaemonProcess(background bool) error {
	if background {
		configureBackgroundLogging()
	}

	dataDir, err := config.DataDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return err
	}

	tokenPath, err := config.TokenPath()
	if err != nil {
		return err
	}
	token, err := daemon.LoadOrCreateToken(tokenPath)
	if err != nil {
		return err
	}

	sessionsDir, err := config.SessionsDir()
	if err != nil {
		return err
	}
	manager, err := daemon.NewSessionManager(sessionsDir)
	if err != nil {
		return err
	}

	workspacesPath, err := config.WorkspacesPath()
	if err != nil {
		return err
	}
	statePath, err := config.StatePath()
	if err != nil {
		return err
	}
	sessionsMetaPath, err := config.SessionsMetaPath()
	if err != nil {
		return err
	}
	sessionsIndexPath, err := config.SessionsIndexPath()
	if err != nil {
		return err
	}
	approvalsPath, err := config.ApprovalsPath()
	if err != nil {
		return err
	}
	notesPath, err := config.NotesPath()
	if err != nil {
		return err
	}
	workspaceStore := store.NewFileWorkspaceStore(workspacesPath)
	appStateStore := store.NewFileAppStateStore(statePath)
	sessionMetaStore := store.NewFileSessionMetaStore(sessionsMetaPath)
	sessionIndexStore := store.NewFileSessionIndexStore(sessionsIndexPath)
	approvalStore := store.NewFileApprovalStore(approvalsPath)
	noteStore := store.NewFileNoteStore(notesPath)
	stores := &daemon.Stores{
		Workspaces:  workspaceStore,
		Worktrees:   workspaceStore,
		Groups:      workspaceStore,
		AppState:    appStateStore,
		SessionMeta: sessionMetaStore,
		Sessions:    sessionIndexStore,
		Approvals:   approvalStore,
		Notes:       noteStore,
	}
	coreCfg, err := config.LoadCoreConfig()
	if err != nil {
		return err
	}
	addr := coreCfg.DaemonAddress()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	d := daemon.New(addr, token, buildVersion(), manager, stores)
	return d.Run(ctx)
}

func killDaemonWithFactory(newClient clientFactory) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	client, err := newClient()
	if err != nil {
		return err
	}
	if err := client.ShutdownDaemon(ctx); err == nil {
		return nil
	} else {
		var apiErr *controlclient.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			return nil
		}
		if isDaemonUnavailable(err) {
			return nil
		}
	}
	resp, err := client.Health(ctx)
	if err != nil {
		if isDaemonUnavailable(err) {
			return nil
		}
		return err
	}
	if resp == nil || resp.PID <= 0 {
		return nil
	}
	return terminatePID(resp.PID)
}

func terminatePID(pid int) error {
	if pid <= 0 {
		return errors.New("invalid pid")
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Signal(syscall.SIGTERM)
}

func isDaemonUnavailable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "connection refused") || strings.Contains(lower, "connect: connection refused")
}

func configureBackgroundLogging() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	dataDir, err := config.DataDir()
	if err != nil {
		return
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return
	}
	logPath := filepath.Join(dataDir, "daemon.log")
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	log.SetOutput(file)
}
