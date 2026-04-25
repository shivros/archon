## 1. Start Contract

- [x] 1.1 Audit `cmd/archon/command_start.go` and the client adapter so `archon start` matches the documented flag, argument, and stdout contract.
- [x] 1.2 Add or tighten command tests for missing-provider validation, ordered forwarding of args/tags/env, and single-line session-id output.
- [x] 1.3 Update CLI help text and README examples so the documented invocation matches the locked contract.

## 2. Kill Contract

- [x] 2.1 Audit `cmd/archon/command_kill.go` so positional session-id validation and success/error behavior match the spec.
- [x] 2.2 Add or tighten tests for missing session id, daemon-call wiring, and `ok` success output.

## 3. Tail Snapshot Contract

- [x] 3.1 Audit `cmd/archon/command_tail.go` so snapshot mode always emits a JSON array followed by a newline.
- [x] 3.2 Lock the `--lines` default and explicit override behavior with command tests.
- [x] 3.3 Ensure snapshot-mode documentation clearly distinguishes it from `--follow`, which is covered in the separate follow-mode change.

## 4. Verification

- [x] 4.1 Run `go test ./cmd/archon/...`.
- [x] 4.2 Run `openspec validate add-cli-session-lifecycle-basics`.
