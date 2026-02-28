## Summary

- What changed?
- Why now?

## No Behavior Changes Checklist (Phase 0)

- [ ] I only made structural/documentation/test changes, not product behavior changes.
- [ ] Streaming behavior is unchanged (log stream, events stream, items stream).
- [ ] Compose/send flow behavior is unchanged (input handling, pending send states).
- [ ] Session selection/load behavior is unchanged (selection debounce, history/stream loading).
- [ ] Approval visibility behavior is unchanged (approvals list + stream-driven approvals).
- [ ] `go test ./...` passes locally.
- [ ] `go test -race ./internal/app ./internal/daemon` passes locally.
- [ ] Provider integration tests were not regressed (Codex + Claude + OpenCode suites; opt-out via `ARCHON_*_INTEGRATION=disabled`).

## Validation Notes

- Key tests run:
- Known risks / follow-ups:
