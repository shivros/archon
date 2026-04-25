## 1. Badge Resolution Contract

- [x] 1.1 Audit sidebar badge resolution so provider-specific defaults, unknown-provider fallback, and selective override application match the spec.
- [x] 1.2 Add or tighten tests for default badges, normalized override keys, partial overrides, and unknown-provider fallback behavior.

## 2. App-State Persistence

- [x] 2.1 Audit app-state load/save behavior so `provider_badges` round-trips through `~/.archon/state.json`.
- [x] 2.2 Add or tighten persistence tests for saved badge overrides.

## 3. Documentation And Verification

- [x] 3.1 Update `README.md` if needed so provider badge customization docs match the locked contract.
- [x] 3.2 Run focused tests for `internal/app` and `internal/store`.
- [x] 3.3 Run `openspec validate add-session-provider-badge-overrides`.
