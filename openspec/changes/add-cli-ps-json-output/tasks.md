## 1. CLI Implementation

- [x] 1.1 Add a `--json` boolean flag to the `flag.FlagSet` in `cmd/archon/command_ps.go` with a clear usage string (e.g., "emit machine-readable JSON array of sessions").
- [x] 1.2 Branch on the flag after the `client.ListSessions(ctx)` call: when `--json` is set, marshal the `[]*types.Session` slice with `encoding/json`'s `MarshalIndent` using two-space indentation, write the result to `c.stdout`, and append a single `\n`.
- [x] 1.3 When `--json` is set and the returned slice is `nil` or empty, emit exactly `[]\n` on stdout (do not emit `null`).
- [x] 1.4 When `--json` is NOT set, keep the existing call to `printSessions(c.stdout, sessions)` unchanged.
- [x] 1.5 Ensure any future selection-affecting flags on `ps` flow through identically regardless of `--json` (for now this means the JSON branch uses the same `ListSessions` call; do not short-circuit it).

## 2. Tests

- [x] 2.1 In `cmd/archon/commands_test.go` (or a new `command_ps_test.go` if that is closer to the file's style), add a test that runs `ps --json` against the existing `fakeCommandClient` with a small fixture list of sessions and asserts: output is valid JSON, decodes into `[]map[string]any`, each element has `id`, `status`, `provider`, `pid`, and `title` fields.
- [x] 2.2 Add a test that runs `ps --json` with zero sessions and asserts the output is exactly `[]\n`.
- [x] 2.3 Add a test that runs `ps` without `--json` on the same fixture and asserts the tab-separated table is unchanged byte-for-byte from the current golden output (use a snapshot or equivalent).
- [x] 2.4 Add a comment above the `--json` test noting that the asserted field set is the CLI contract and changes to it need to be intentional.

## 3. Documentation

- [x] 3.1 Update `README.md`'s `archon ps` reference (in the commands table or the CLI examples) to mention `--json` and note that it is the machine-readable contract.
- [x] 3.2 Update `cmd/archon/command_ps.go`'s `fs.Usage` (if set) or the flag usage string so `archon ps -h` clearly documents the `--json` flag.

## 4. Verification

- [x] 4.1 Run `go build ./...` and confirm no regressions.
- [x] 4.2 Run `go test ./cmd/archon/...` and confirm new tests pass and existing tests are unchanged.
- [x] 4.3 Manual smoke: start the daemon, create at least one session, run `archon ps` and `archon ps --json`, pipe the JSON through `jq '.[].id'` to confirm it's consumable.
