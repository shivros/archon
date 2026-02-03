package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"control/internal/app"
	controlclient "control/internal/client"
	"control/internal/config"
	"control/internal/daemon"
	"control/internal/store"
	"control/internal/types"
)

const usageText = `control is a placeholder CLI.

Usage:
  control <command> [flags]

Commands:
  daemon   run background daemon
  ps       list sessions
  start    start a session
  kill     kill a session
  tail     show recent session output
  ui       run terminal UI (placeholder)
  help     show help

Flags:
  -h, --help   show help

Daemon flags:
  --background    run in background (logs to file)
  --force         stop any running daemon before starting
  --kill          stop any running daemon and exit

Examples:
  control ps
  control start --provider codex --cwd . -- --help
  control tail <id> --lines 200
`

const version = "dev"

func printUsage() {
	fmt.Fprint(os.Stderr, usageText)
}

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		printUsage()
		return
	}

	switch args[0] {
	case "-h", "--help", "help":
		printUsage()
		return
	case "daemon":
		exitOnErr("daemon", runDaemonCommand(args[1:]))
	case "ui":
		exitOnErr("ui", runUI(args[1:]))
	case "ps":
		exitOnErr("ps", runPS(args[1:]))
	case "start":
		exitOnErr("start", runStart(args[1:]))
	case "kill":
		exitOnErr("kill", runKill(args[1:]))
	case "tail":
		exitOnErr("tail", runTail(args[1:]))
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", args[0])
		printUsage()
		os.Exit(2)
	}
}

func runDaemonCommand(args []string) error {
	fs := flag.NewFlagSet("daemon", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	background := fs.Bool("background", false, "run in background (logs to file)")
	kill := fs.Bool("kill", false, "stop any running daemon and exit")
	force := fs.Bool("force", false, "stop any running daemon before starting")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *kill {
		return killDaemon()
	}
	if *force {
		if err := killDaemon(); err != nil {
			return err
		}
	}
	return runDaemon(*background)
}

func runDaemon(background bool) error {
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
	keymapPath, err := config.KeymapPath()
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
	workspaceStore := store.NewFileWorkspaceStore(workspacesPath)
	appStateStore := store.NewFileAppStateStore(statePath)
	keymapStore := store.NewFileKeymapStore(keymapPath)
	sessionMetaStore := store.NewFileSessionMetaStore(sessionsMetaPath)
	sessionIndexStore := store.NewFileSessionIndexStore(sessionsIndexPath)
	stores := &daemon.Stores{
		Workspaces:  workspaceStore,
		Worktrees:   workspaceStore,
		AppState:    appStateStore,
		Keymap:      keymapStore,
		SessionMeta: sessionMetaStore,
		Sessions:    sessionIndexStore,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	d := daemon.New("127.0.0.1:7777", token, buildVersion(), manager, stores)
	return d.Run(ctx)
}

func killDaemon() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	client, err := controlclient.New()
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

func runPS(args []string) error {
	fs := flag.NewFlagSet("ps", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx := context.Background()
	client, err := controlclient.New()
	if err != nil {
		return err
	}
	if err := client.EnsureDaemon(ctx); err != nil {
		return err
	}
	sessions, err := client.ListSessions(ctx)
	if err != nil {
		return err
	}

	printSessions(sessions)
	return nil
}

func runStart(args []string) error {
	fs := flag.NewFlagSet("start", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	provider := fs.String("provider", "", "provider name")
	cwd := fs.String("cwd", "", "working directory")
	cmd := fs.String("cmd", "", "command override (custom provider)")
	title := fs.String("title", "", "session title")
	var tags stringList
	var envs stringList
	fs.Var(&tags, "tag", "tag (repeatable)")
	fs.Var(&envs, "env", "environment variable KEY=VALUE (repeatable)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cmdArgs := fs.Args()
	if *provider == "" {
		return errors.New("provider is required")
	}

	ctx := context.Background()
	client, err := controlclient.New()
	if err != nil {
		return err
	}
	if err := client.EnsureDaemon(ctx); err != nil {
		return err
	}

	session, err := client.StartSession(ctx, controlclient.StartSessionRequest{
		Provider: *provider,
		Cmd:      *cmd,
		Cwd:      *cwd,
		Args:     cmdArgs,
		Env:      envs,
		Title:    *title,
		Tags:     tags,
	})
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout, session.ID)
	return nil
}

func runKill(args []string) error {
	fs := flag.NewFlagSet("kill", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return errors.New("kill requires a session id")
	}
	id := fs.Arg(0)

	ctx := context.Background()
	client, err := controlclient.New()
	if err != nil {
		return err
	}
	if err := client.EnsureDaemon(ctx); err != nil {
		return err
	}
	if err := client.KillSession(ctx, id); err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout, "ok")
	return nil
}

func runTail(args []string) error {
	fs := flag.NewFlagSet("tail", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	lines := fs.Int("lines", 200, "number of lines to fetch")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return errors.New("tail requires a session id")
	}
	id := fs.Arg(0)

	ctx := context.Background()
	client, err := controlclient.New()
	if err != nil {
		return err
	}
	if err := client.EnsureDaemon(ctx); err != nil {
		return err
	}
	resp, err := client.TailItems(ctx, id, *lines)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(os.Stdout).Encode(resp.Items); err != nil {
		return err
	}
	return nil
}

func runUI(args []string) error {
	fs := flag.NewFlagSet("ui", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	restartDaemon := fs.Bool("restart-daemon", false, "restart daemon if version mismatch")
	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx := context.Background()
	client, err := controlclient.New()
	if err != nil {
		return err
	}
	if err := client.EnsureDaemonVersion(ctx, buildVersion(), *restartDaemon); err != nil {
		return err
	}
	return app.Run(client)
}

func printSessions(sessions []*types.Session) {
	writer := tabwriter.NewWriter(os.Stdout, 0, 8, 2, ' ', 0)
	fmt.Fprintln(writer, "ID\tSTATUS\tPROVIDER\tPID\tTITLE")
	for _, session := range sessions {
		pid := "-"
		if session.PID > 0 {
			pid = fmt.Sprintf("%d", session.PID)
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n", session.ID, session.Status, session.Provider, pid, session.Title)
	}
	_ = writer.Flush()
}

type stringList []string

func (s *stringList) String() string {
	return strings.Join(*s, ",")
}

func (s *stringList) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func exitOnErr(label string, err error) {
	if err == nil {
		return
	}
	fmt.Fprintf(os.Stderr, "%s error: %v\n", label, err)
	os.Exit(1)
}

func buildVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		var revision string
		var modified string
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				revision = setting.Value
			case "vcs.modified":
				modified = setting.Value
			}
		}
		if revision != "" {
			if modified == "true" {
				return revision + "-dirty"
			}
			return revision
		}
	}

	exe, err := os.Executable()
	if err == nil {
		file, err := os.Open(exe)
		if err == nil {
			defer file.Close()
			hasher := sha256.New()
			if _, err := io.Copy(hasher, file); err == nil {
				sum := hasher.Sum(nil)
				return fmt.Sprintf("bin-%x", sum[:6])
			}
		}
	}

	return version
}
