## 1. Login Contract

- [x] 1.1 Audit `cmd/archon/command_login.go` so fallback URL/code output, browser-launch behavior, and polling semantics match the spec.
- [x] 1.2 Add or tighten tests for `--no-browser`, browser-open warning behavior, `authorization_pending`, `slow_down`, and unexpected status handling.

## 2. WhoAmI And Logout Contract

- [x] 2.1 Audit `cmd/archon/command_whoami.go` so linked and unlinked output matches the documented human-readable contract.
- [x] 2.2 Audit `cmd/archon/command_logout.go` so daemon messages are surfaced exactly once for both full and partial unlink outcomes.
- [x] 2.3 Add or tighten tests for `not logged in`, linked identity rendering, and partial logout messaging.

## 3. Documentation And Verification

- [x] 3.1 Update `README.md` and command help text so cloud-auth docs match the locked CLI contract.
- [x] 3.2 Run `go test ./cmd/archon/...`.
- [x] 3.3 Run `openspec validate add-cli-cloud-auth`.
