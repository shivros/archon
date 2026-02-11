package main

import (
	"fmt"
	"os"
)

const usageText = `archon is a placeholder CLI.

Usage:
  archon <command> [flags]

Commands:
  daemon   run background daemon
  config   print configuration (effective or defaults)
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
  archon ps
  archon config --scope core --format toml
  archon start --provider codex --cwd . -- --help
  archon tail <id> --lines 200
`

func printUsage() {
	fmt.Fprint(os.Stderr, usageText)
}

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		printUsage()
		return
	}

	wiring := defaultCommandWiring(os.Stdout, os.Stderr)
	commands := buildCommands(wiring)

	switch args[0] {
	case "-h", "--help", "help":
		printUsage()
		return
	}

	runner, ok := commands[args[0]]
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", args[0])
		printUsage()
		os.Exit(2)
	}
	exitOnErr(args[0], runner.Run(args[1:]), wiring.stderr)
}
