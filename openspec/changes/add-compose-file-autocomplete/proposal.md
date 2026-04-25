## Why

Compose `@` file autocomplete is one of Archon's signature cross-provider UI behaviors, and the daemon/client stack already exposes a dedicated file-search API to support it. Despite that, there is no OpenSpec contract for either the compose-side behavior or the underlying file-search routes, leaving a large user-facing feature surface undocumented.

## What Changes

- Add a UI contract for compose `@` file autocomplete, including supported-provider behavior, unsupported-provider fallback, and textual mention insertion.
- Add an API contract for provider-agnostic file-search creation, updates, streaming results, and teardown.
- Lock the V1 rule that compose insertion is plain-text `@path` mention text rather than a structured provider payload.
- Capture the unsupported-provider error contract that downstream clients already rely on.

## Capabilities

### New Capabilities
- `compose-file-autocomplete`: Defines the user-facing compose `@` experience across supported and unsupported providers.
- `file-search-api`: Defines the daemon/client file-search transport contract used by compose autocomplete.

### Modified Capabilities

## Impact

- **Affected code:**
  - `internal/app/compose_*`
  - `internal/app/model.go`
  - `internal/client/client.go`
  - `internal/client/sse.go`
  - `internal/daemon/api_file_search_handlers.go`
  - `internal/daemon/file_search_service.go`
  - `internal/providers/registry.go`
  - `README.md`
  - `docs/architecture.md`
- **Affected behavior:** No new feature is introduced; this change specifies and hardens the existing compose/file-search contract.
- **Dependencies:** Existing provider file-search adapters for Codex, OpenCode, and Kilo Code.
- **Out of scope for this change:**
  - Structured provider-specific compose payload insertion
  - File-search support for providers that do not currently implement it
  - Non-compose consumers of the file-search API
