## Why

Provider badge overrides are documented in the README and already persisted in `~/.archon/state.json`, but there is no OpenSpec contract for that behavior yet. That leaves a small but user-visible customization surface undocumented even though the sidebar and state-store tests already treat it as stable behavior.

## What Changes

- Add a UI contract for provider badge defaults shown in the session sidebar.
- Add a contract for per-provider badge overrides loaded from `provider_badges` in `~/.archon/state.json`.
- Lock the current normalization and fallback behavior for override keys, partial overrides, and unknown providers.
- Capture persistence expectations so badge overrides round-trip through app state storage.

## Capabilities

### New Capabilities
- `session-provider-badges`: Defines default provider badge rendering, override application, and app-state persistence for sidebar session rows.

### Modified Capabilities

## Impact

- **Affected code:**
  - `internal/app/sidebar.go`
  - `internal/app/sidebar_controller.go`
  - `internal/app/sidebar_test.go`
  - `internal/store/state_store.go`
  - `internal/store/state_store_test.go`
  - `internal/types/app_state.go`
  - `README.md`
- **Affected behavior:** No new end-user feature is introduced; this change formalizes the existing sidebar badge customization contract.
- **Dependencies:** Existing app-state persistence and provider normalization helpers.
- **Out of scope for this change:**
  - New badge fields beyond prefix and color
  - Theme-specific badge customization beyond the existing default theme colors
  - Non-sidebar uses of provider identity styling
