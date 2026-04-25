## Why

Archon currently ships with Codex, Claude, OpenCode/Kilocode, Gemini, and a custom runtime, but has no integration for Hermes (Nous Research's agent). Hermes publishes a supported `hermes acp` stdio entry point that speaks the Agent Client Protocol (ACP) — the same protocol Zed, Gemini CLI, and other clients are standardizing on — so we can add Hermes by adopting ACP rather than reverse-engineering a bespoke wire format.

Building an ACP client now is a strategic investment: the client layer is reusable for any future ACP-speaking provider, and the Codex adapter already demonstrates the stdio-JSON-RPC subprocess model this fits into.

## What Changes

- Add a new `RuntimeACP` runtime in `internal/providers/registry.go` and register Hermes as a provider definition under that runtime.
- Introduce a reusable `internal/daemon/acp/` package that handles ACP's newline-delimited JSON-RPC framing, request/response correlation, notification fan-out, graceful shutdown, and interrupt — independent of any single provider.
- Add `internal/daemon/provider_hermes.go` implementing the existing `Provider` interface on top of the ACP client package, modeled after `provider_codex.go`.
- Add `internal/daemon/transcriptadapters/hermes_adapter.go` mapping ACP `session/update`, tool-call, and turn events to canonical `transcriptdomain` events, and register it in `adapters.go`.
- Add `CoreHermesProviderConfig` to `internal/config/settings.go` (command, args, env overrides), shaped in parallel with `CoreCodexProviderConfig` and `CoreClaudeProviderConfig`.
- Wire a Hermes factory entry into `internal/daemon/provider.go` keyed on `RuntimeACP`.
- Advertise the Hermes `Capabilities` to match what ACP + the Hermes server actually implement — `SupportsEvents`, `SupportsInterrupt`, and `SupportsGuidedWorkflowDispatch` are assumed; `SupportsApprovals` and `SupportsFileSearch` are verified during design against the spec and Hermes' implementation, and scoped down rather than faked if not supported.

This change targets Codex-level parity (streaming transcript events, interrupt, optional approvals) — not Claude-level feature depth.

## Capabilities

### New Capabilities
- `acp-runtime`: Reusable ACP client substrate for stdio JSON-RPC agents — framing, request/response correlation, notification fan-out, interrupt, and graceful shutdown, usable by any ACP-speaking provider.
- `hermes-provider`: Archon supports Hermes as a first-class provider, started as `hermes acp`, with streaming transcript events, session interrupt, and capability reporting that reflects what Hermes' ACP server actually implements.

### Modified Capabilities

## Impact

- **Affected code:**
  - `internal/providers/registry.go` — new `RuntimeACP` constant and Hermes definition.
  - `internal/daemon/provider.go` — new factory entry for `RuntimeACP`.
  - `internal/daemon/provider_hermes.go` — new file.
  - `internal/daemon/acp/` — new package (framing, client, types).
  - `internal/daemon/transcriptadapters/hermes_adapter.go` — new file.
  - `internal/daemon/transcriptadapters/adapters.go` — register Hermes adapter.
  - `internal/config/settings.go` — new `CoreHermesProviderConfig`, wired into `CoreProvidersConfig` and command resolution.
  - `internal/daemon/session_service.go` — sender/interrupter dispatch for the new runtime if the existing Runtime-keyed dispatch does not already fan out generically.
- **Affected behavior:** New sessions can select the Hermes provider; existing providers are untouched.
- **Dependencies:** No new Go module dependencies expected — the ACP client is implemented on top of `os/exec` and `encoding/json`. The runtime dependency `hermes` is user-provided on `$PATH`, same as `codex` and `claude`.
- **Out of scope for this change:**
  - Migrating the existing Codex or OpenCode runtimes onto the new ACP client package.
  - UI changes beyond what is required to render Hermes transcripts correctly.
  - A separate HTTP runtime for Hermes (the `hermes dashboard` server is a human UI, not a programmatic API).
