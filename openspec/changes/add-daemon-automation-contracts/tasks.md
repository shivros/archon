## 1. Title Generation Contract

- [x] 1.1 Audit async title-generation enablement and queueing so session/workflow creation paths match the new spec.
- [x] 1.2 Add or tighten tests for compare-and-set updates, locked-title skipping, restart-safe best-effort behavior, and metadata-event publication.

## 2. Notification Contract

- [x] 2.1 Audit notification policy resolution so session overrides, worktree overrides, and global defaults compose in the documented precedence order.
- [x] 2.2 Audit notification dispatch so `auto`, explicit methods, script-command payload delivery, and dedupe-window behavior match the spec.
- [x] 2.3 Add or tighten tests for script stdin/env delivery, fallback order, unknown methods, and duplicate suppression.

## 3. Documentation And Verification

- [x] 3.1 Update `README.md` so notification and title-generation docs match the locked contracts.
- [x] 3.2 Run focused tests for `internal/daemon` and `internal/config`.
- [x] 3.3 Run `openspec validate add-daemon-automation-contracts`.
