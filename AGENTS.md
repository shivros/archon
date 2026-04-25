# Archon — Agent Instructions

## Project Overview

Archon is a Go CLI+daemon that manages agentic coding sessions. It starts, observes, and controls sessions backed by providers like Codex and Claude Code. The binary is built from `cmd/archon/` and the daemon runs in the background via `archon daemon -background`.

## Build & Test

```bash
go build -o /tmp/archon ./cmd/archon/          # build
go test ./cmd/archon/... -count=1               # CLI tests
go test ./... -count=1                          # full suite (some tests need OpenAI creds, ignore those failures)
go vet ./...                                    # static checks
```

## Daemon Lifecycle

```bash
archon daemon -kill                             # stop the daemon
archon daemon -background                       # start (blocks, run in background)
archon ps --json                                # health check
```

When replacing the binary, kill the daemon first (the running process locks the file), then `mv` the old binary aside before copying the new one in:

```bash
archon daemon -kill
sleep 1
mv ~/.local/bin/archon ~/.local/bin/archon.old
cp /tmp/archon ~/.local/bin/archon
archon daemon -background   # run as background process
```

## LLM-Testable CLI Surfaces

A core design principle: **every CLI surface should be testable by an LLM agent without human intervention.** This means:

- Commands should produce machine-readable output (JSON, NDJSON, plain IDs on a single line).
- Errors go to stderr as single-line messages; stdout is clean for piping.
- Flags work in any order (see the Go flag reordering pattern below).
- When implementing or modifying a command, always smoke-test it against the live daemon yourself. Start sessions, send messages, tail output, verify the round-trip works end-to-end.

You should exercise the full lifecycle autonomously: build, install, restart daemon, run commands, inspect output. Don't ask the user to do things you can do yourself.

## Go Flag Ordering

Go's `flag` package stops parsing at the first non-flag argument. Since users (and LLMs) naturally write `archon tail ID --follow` instead of `archon tail --follow ID`, every command with positional args must reorder flags before positionals. The pattern:

```go
// valueFlags maps flag names that take a value (not bools).
func reorderXxxFlags(args []string) []string {
    valueFlags := map[string]bool{"lines": true, "stream": true, ...}
    var flags, positionals []string
    for i := 0; i < len(args); i++ {
        a := args[i]
        if strings.HasPrefix(a, "--") {
            name := strings.TrimPrefix(a, "--")
            if idx := strings.IndexByte(name, '='); idx >= 0 {
                flags = append(flags, a)
                continue
            }
            flags = append(flags, a)
            if valueFlags[name] && i+1 < len(args) {
                i++
                flags = append(flags, args[i])
            }
        } else {
            positionals = append(positionals, a)
        }
    }
    return append(flags, positionals...)
}
```

Apply this to any new command that mixes flags with positional args.

## OpenSpec Workflow

We use OpenSpec (spec-driven development) for all changes. The workflow:

1. `openspec list --json` — see all changes and their status
2. `openspec status --change <name> --json` — check a specific change
3. `openspec instructions apply --change <name> --json` — get apply instructions
4. Read `design.md` and `tasks.md` from `openspec/changes/<name>/`
5. Implement, marking tasks `[x]` in `tasks.md` as you go
6. Build, test, smoke-test against live daemon
7. Verify with `openspec status --change <name> --json` (should show `isComplete: true`)

Key paths:
- `openspec/changes/<name>/design.md` — design spec
- `openspec/changes/<name>/tasks.md` — implementation checklist
- `openspec/changes/<name>/proposal.md` — original proposal
- `openspec/changes/<name>/specs/` — delta specs

## Command Architecture

- `cmd/archon/commands.go` — command dispatch map (`buildCommands`)
- `cmd/archon/client_adapter.go` — `sessionCommandClient` interface + `controlClientAdapter`
- `cmd/archon/command_*.go` — one file per command
- `cmd/archon/commands_test.go` — all tests, using `fakeCommandClient`
- `cmd/archon/main.go` — top-level help text and examples
- `internal/client/` — daemon client library (SSE, REST)
- `internal/daemon/` — daemon-side HTTP handlers and session management
- `internal/types/` — shared types (`Session`, `LogEvent`, etc.)

When adding a new command:
1. Add `SendMessage`-equivalent to `sessionCommandClient` interface
2. Implement on `controlClientAdapter`
3. Add stub fields + method to `fakeCommandClient`
4. Create `command_<name>.go`
5. Register in `buildCommands` (commands.go)
6. Add to help text (main.go)
7. Write tests in `commands_test.go`

## Testing Patterns

Tests use the `fakeCommandClient` which stubs all daemon calls. Key conventions:

- Factory function: `func fixedXxxFactory(fake *fakeCommandClient) sessionClientFactory`
- Check `fake.xxxCalls` for call count assertions
- Check `fake.xxxReq` / `fake.xxxIDArg` for argument assertions
- Stdout/stderr are `*bytes.Buffer` for output assertions
- Commands accept `io.Reader` for stdin (test with `bytes.NewReader`)

## Known Gaps

- Some daemon endpoints exist but the CLI doesn't exercise them yet (e.g., interrupt session).
- The `tail --follow` SSE streaming works end-to-end; the daemon already has the SSE handler.
- Provider-level behavior (e.g., whether Codex processes follow-up messages from `send`) may vary — the CLI surface is correct regardless.
