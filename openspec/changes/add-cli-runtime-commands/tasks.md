## 1. Config Inspection Contract

- [x] 1.1 Audit `cmd/archon/command_config.go` so default/effective output, `--default`, `--format`, and `--scope` behavior match the new spec.
- [x] 1.2 Add or tighten tests for invalid formats/scopes, scope-specific payload shapes, and malformed-user-file bypass in `--default` mode.

## 2. Daemon Control Contract

- [x] 2.1 Audit `cmd/archon/command_daemon.go` so foreground start, `--background`, `--kill`, `--force`, and flag precedence match the documented contract.
- [x] 2.2 Add or tighten tests for stop-only and force-restart paths.

## 3. UI Launch Contract

- [x] 3.1 Audit `cmd/archon/command_ui.go` so version gating, restart forwarding, and `--ignore-daemon-mismatch` behavior match the spec.
- [x] 3.2 Add or tighten tests for default version checks, restart forwarding, and ignore-mismatch behavior.

## 4. Documentation And Verification

- [x] 4.1 Update usage/help text and `README.md` so runtime-management docs match the locked command contracts.
- [x] 4.2 Run `go test ./cmd/archon/...`.
- [x] 4.3 Run `openspec validate add-cli-runtime-commands`.
