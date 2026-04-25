package main

import (
	"fmt"
	"io"
	"os"
)

const usageText = `archon is a placeholder CLI.

Usage:
  archon <command> [flags]

Commands:
  daemon   run background daemon
  config   print configuration (effective or defaults)
  login    link this Archon daemon to Archon Cloud
  whoami   show current Archon Cloud link status
  logout   unlink this Archon daemon from Archon Cloud
  ps       list sessions
  session  show full details for one session
  start    start a session
  kill     kill a session
  interrupt stop the in-flight turn for a session
  send     send a message to a session
  tail     show recent session output (use --follow to stream live)
  approvals list pending approvals for a session
  approve   respond to a pending approval
  ui       run terminal UI
  version  print CLI build metadata
  help     show help

Flags:
  -h, --help   show help
  -v, --version   show version

Daemon flags:
  --background    run in background (logs to file)
  --force         stop any running daemon before starting
  --kill          stop any running daemon and exit

UI flags:
  --restart-daemon         restart daemon before launching UI
  --ignore-daemon-mismatch start UI even if daemon version mismatches

Examples:
  archon ps
  archon session abc123
  archon session abc123 --format human
  archon config --scope core --format toml
  archon start --provider codex --cwd . -- --help
  archon tail <id> --lines 200
  archon tail <id> --follow --stream stderr
  archon send <id> "hello"
  archon send <id> --input-items items.json --json
  archon interrupt <id>
  archon approvals <id>
  archon approve <id> --request-id 1 --decision allow_once
`

var rootCommandAliases = map[string]string{
	"-h":        "help",
	"--help":    "help",
	"-v":        "version",
	"--version": "version",
}

func printUsage() {
	printUsageTo(os.Stderr)
}

func resolveRootCommandName(value string) string {
	if alias, ok := rootCommandAliases[value]; ok {
		return alias
	}
	return value
}

func main() {
	wiring := defaultCommandWiring(os.Stdout, os.Stderr)
	exitCode := runCLI(os.Args[1:], wiring)
	if exitCode != 0 {
		os.Exit(exitCode)
	}
}

func runCLI(args []string, wiring commandWiring) int {
	return runCLIWithCommands(args, wiring, buildCommands(wiring))
}

func runCLIWithCommands(args []string, wiring commandWiring, commands map[string]commandRunner) int {
	stderr := wiring.stderr
	if stderr == nil {
		stderr = os.Stderr
	}

	if len(args) == 0 {
		printUsageTo(stderr)
		return 0
	}

	commandName := resolveRootCommandName(args[0])
	if commandName == "help" {
		printUsageTo(stderr)
		return 0
	}

	runner, ok := commands[commandName]
	if !ok {
		_, _ = fmt.Fprintf(stderr, "unknown command: %s\n\n", args[0])
		printUsageTo(stderr)
		return 2
	}
	if err := runner.Run(args[1:]); err != nil {
		_, _ = fmt.Fprintf(stderr, "%s error: %v\n", commandName, err)
		return 1
	}
	return 0
}

func printUsageTo(output io.Writer) {
	_, _ = fmt.Fprint(output, usageText)
}
