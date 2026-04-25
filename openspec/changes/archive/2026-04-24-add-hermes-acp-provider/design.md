## Context

Archon registers providers through `internal/providers/registry.go` (runtime + capabilities) and runs them through `internal/daemon/provider.go` (the `Provider` interface and a runtime-keyed factory). The Codex runtime spawns `codex app-server`, speaks a bespoke stdio JSON-RPC dialect via `internal/daemon/provider_codex.go`, and maps its events to canonical `transcriptdomain` events in `internal/daemon/transcriptadapters/codex_adapter.go`.

Hermes (Nous Research's agent) exposes a public `hermes acp` subcommand that implements the Agent Client Protocol (ACP, `https://agentclientprotocol.com/`). ACP is also implemented by Zed and targeted by Gemini CLI and other agents. Verified properties relevant to this change:

- **Transport** — stdio JSON-RPC 2.0, newline-delimited UTF-8, no embedded newlines, stderr reserved for logs (ACP spec, `docs/protocol/transports.mdx`).
- **Method surface** — `initialize`, `authenticate`, `session/new`, `session/load`, `session/prompt`, `session/set_mode`, `session/cancel` (notification). Notifications from agent: `session/update` (with `sessionUpdate` variants `agent_message_chunk`, `user_message_chunk`, `agent_thought_chunk`, `tool_call`, `tool_call_update`, `plan`, `available_commands_update`, `current_mode_update`). Agent-to-client requests: `session/request_permission` (baseline), plus optional `fs/read_text_file`, `fs/write_text_file`, `terminal/*` gated by client-advertised capabilities (ACP `docs/protocol/overview.mdx`, `initialization.mdx`, `prompt-turn.mdx`).
- **Stop reasons** — `end_turn`, `max_tokens`, `max_turn_requests`, `refusal`, `cancelled`.
- **Hermes implementation** — launches via `hermes acp`, reserves stdout for ACP traffic, stderr for logs, supports `session/new`, `session/load`, `session/cancel`, `session/prompt`, and routes dangerous-terminal approvals through `session/request_permission` with `allow_once`/`allow_always`/`deny` (Hermes `website/docs/user-guide/features/acp.md`, `developer-guide/acp-internals.md`). Sessions are **process-local** on the Hermes side — they do not persist across Hermes process restarts.

## Goals / Non-Goals

**Goals:**
- Add Hermes as an archon provider usable from the existing session UI with Codex-level parity: streaming transcript, interrupt, and approvals when Hermes issues them.
- Introduce a self-contained, provider-agnostic ACP client package at `internal/daemon/acp/` that other ACP-speaking providers can adopt later without re-implementing framing, correlation, or notification fan-out.
- Preserve archon's existing provider lifecycle contract (`Provider`/`providerProcess` from `internal/daemon/provider.go`), registry and capability model (`internal/providers/registry.go`), and transcript adapter registry (`internal/daemon/transcriptadapters/adapters.go`) without rewriting them.
- Keep the change reviewable in a single PR.

**Non-Goals:**
- Migrating `codex`, `claude`, or `opencode_server` runtimes onto the new ACP client.
- Adding an HTTP transport (the `hermes dashboard` server is a human UI, not a programmatic API; ACP itself lists Streamable HTTP as a draft).
- Implementing client-side `fs/*` or `terminal/*` handlers. Hermes has its own in-process file and terminal tools; archon is a daemon, not an editor, and does not need to advertise these capabilities.
- Cross-daemon-restart session resume (see Risks/Trade-offs below).
- Authentication flows. Hermes resolves credentials from `~/.hermes/.env`/`~/.hermes/config.yaml` on its own process and does not require ACP-side `authenticate` from archon.

## Decisions

### 1. Introduce a reusable `internal/daemon/acp/` package
Create a package `acp` with no provider-specific knowledge. Public surface (approximately):

- `Client` struct — owns a `*exec.Cmd`, stdin/stdout pipes, request-correlation map, notification subscribers list, and a background read loop. Construct via `acp.Start(ctx, StartOptions)`.
- `StartOptions` — `Command`, `Args`, `Env`, `Cwd`, `StderrSink`, `DebugSink`, `ClientInfo`, `ClientCapabilities`, `ProtocolVersion`, `Handlers` (for incoming agent→client requests).
- `Call(ctx, method, params, out)` — synchronous request/response with ID correlation.
- `Notify(method, params)` — fire-and-forget notification.
- `Subscribe() <-chan acp.Notification` / `Unsubscribe(ch)` — fan-out for incoming agent notifications.
- `RegisterHandler(method, handler)` — for agent→client baseline `session/request_permission` and optional `fs/*`, `terminal/*` methods, returning a JSON-RPC response.
- `Close(ctx)` — closes stdin, waits for the process, kills on timeout.

The package exports strongly-typed request/response/notification structs for the methods archon actually uses (`initialize`, `session/new`, `session/load`, `session/prompt`, `session/cancel`, `session/update`, `session/request_permission`), each tagged with JSON encoding. Unknown `session/update` variants are decoded into a `Raw json.RawMessage` fallback so the client survives protocol extensions.

**Why:** ACP is a standard with multiple implementations (Zed/Gemini/Hermes). Building a reusable substrate up front is cheaper than retrofitting one when a second ACP provider lands, and it keeps `provider_hermes.go` honest about Hermes-specific concerns (command resolution, model selection, capability scoping).

**Alternative considered:** Inlining the framing and JSON-RPC plumbing into `provider_hermes.go` like `provider_codex.go` does. Rejected because Codex's dialect is private to Codex, whereas ACP is a shared protocol — the duplication cost of the next ACP provider would be high.

### 2. Shape the Hermes provider after Codex, not Claude
Model `provider_hermes.go` on `provider_codex.go`: long-running subprocess per archon session, `Start()` spawns the process, initializes ACP, creates a session, and returns a `*providerProcess` whose `ThreadID` holds the ACP session ID and whose `Interrupt` sends `session/cancel`. Each turn becomes a `session/prompt` call; the prompt response carries a stop reason, and intermediate `session/update` notifications stream through the transcript adapter.

**Why:** Codex already uses the stdio-JSON-RPC-subprocess model and integrates with archon's event streaming, interrupt, and sink plumbing. Claude's runtime is a one-shot subprocess with `NoProcess: true` (it uses item-based updates) — a different shape that would not fit ACP's streaming turn model.

**Alternative considered:** A single long-lived Hermes subprocess multiplexed across archon sessions (analogous to how `hermes-acp` inside one editor hosts multiple sessions). Rejected for v1 because the existing `providerProcess` plumbing is session-scoped, and multiplexing would require session-keyed fan-out of notifications that the `session/update` stream already embeds `sessionId` into — cheap to add later, not needed now.

### 3. Advertise only baseline client capabilities
Archon will advertise `clientCapabilities = { fs: { readTextFile: false, writeTextFile: false }, terminal: false }` — i.e., baseline. It will still register a handler for `session/request_permission` because that is baseline and Hermes uses it for terminal-command approvals.

**Why:** Hermes ships its own `read_file`, `write_file`, `patch`, and `terminal` tools that run inside the Hermes process. Archon has no editor-style file or terminal abstraction to offer, and advertising capabilities archon cannot honor would break turns. `session/request_permission` is the one client-side method Hermes actually needs from archon, and archon already has an approvals UI.

**Alternative considered:** Advertising `fs.readTextFile`/`writeTextFile` by pointing at the archon-managed workspace. Rejected because (a) Hermes does not need it — its own file tools already operate in the same cwd — and (b) it would double the surface this change has to implement and test.

### 4. Capability matrix for Hermes
Register Hermes with:
- `SupportsEvents: true` — `session/update` streams tokens, tool calls, and plans.
- `SupportsInterrupt: true` — `session/cancel` is implemented and produces `stopReason: "cancelled"`.
- `SupportsApprovals: true` — Hermes routes terminal-command approvals through `session/request_permission`.
- `SupportsGuidedWorkflowDispatch: true` — Hermes is a valid workflow dispatch target.
- `SupportsFileSearch: false` — ACP has no dedicated file-search verb; Codex's file-search is a Codex extension and its archon plumbing (`provider_codex_file_search.go`) is intentionally not ported.
- `UsesItems: false` — events model matches Codex's event stream, not Claude's item stream.
- `NoProcess: false` — archon owns the subprocess lifecycle.

**Why:** Each flag is justified by an explicit method or notification on Hermes' ACP surface as documented upstream. We scope down where Hermes + ACP do not deliver (file search) rather than faking it and degrading UX silently.

### 5. Session ID handling and resume scope
Treat the ACP `sessionId` returned from `session/new` as the value stored in `providerProcess.ThreadID`. This lets the existing session-service threading (`resolveThreadID` in `session_service.go`) work unchanged.

**Resume:**
- Within a live Hermes subprocess, `session/load` works and we wire it into the session-resume path.
- Across archon daemon restart or Hermes subprocess death, the session is gone. Hermes' own docs call this out ("ACP sessions are process-local from the ACP server's point of view"). Archon will surface this as "session ended" rather than silently restarting with fresh state.

**Why:** Reusing `ThreadID` avoids a schema change to `SessionMeta`. Scoping resume to live-subprocess lifetime matches Hermes' reality and mirrors what an ACP editor like Zed does.

**Alternative considered:** Invoking `hermes --resume`/`--continue` flags to rebuild session state from Hermes' `~/.hermes/state.db`. Rejected because `hermes acp` takes its session state from the ACP adapter's in-memory manager, not from the CLI flags, so this would not actually resume ACP-visible state.

### 6. Config shape parallels Codex
Add `CoreHermesProviderConfig` in `internal/config/settings.go` with `Command`, `DefaultModel`, `Models` (allowlist), `Env` (extra env vars), `Args` (extra args prepended to `acp`). Thread it through `CoreProvidersConfig` and `ProviderCommand("hermes")` so existing command-override plumbing applies uniformly.

**Why:** Same ergonomics as Codex/Claude/OpenCode — one place to override the binary path, model, and env.

### 7. Transcript adapter mapping
`hermes_adapter.go` in `internal/daemon/transcriptadapters/` converts ACP notifications into `transcriptdomain` events:
- `session/update` `agent_message_chunk` → AssistantDelta block
- `session/update` `user_message_chunk` → UserDelta block (for echo/reflection turns)
- `session/update` `agent_thought_chunk` → Thinking block
- `session/update` `tool_call` → ToolCallStarted, classified by ACP `kind` (`file_read`, `file_write`, `patch`, `execute`, `other`)
- `session/update` `tool_call_update` with `status` → ToolCallUpdate / ToolCallCompleted
- `session/update` `plan` → Plan block
- `session/request_permission` → ApprovalPending, mapped to archon's existing approval flow with `once`/`always`/`deny` option set
- `session/prompt` response stopReason → TurnCompleted with reason tag (`end_turn`, `max_tokens`, `max_turn_requests`, `refusal`, `cancelled`)

Register the adapter in `adapters.go` under a `RuntimeACP` factory entry.

**Why:** Mirrors how `codex_adapter.go` classifies Codex events into canonical transcript events. Keeps provider-specific schema translation out of the Hermes provider code and out of the ACP client package.

### 8. Testing
- `internal/daemon/acp/` — unit tests using an in-memory bidirectional `io.Pipe` mock of a cooperating agent process, covering: framing (newline delimiter, UTF-8, long lines), request/response correlation under concurrent callers, notification fan-out, incoming-request handler dispatch, graceful close on `session/cancel`, and misbehavior (malformed JSON, embedded newlines, premature EOF).
- `internal/daemon/transcriptadapters/hermes_adapter_test.go` — table-driven tests covering each `sessionUpdate` variant, stop reasons, and the `session/request_permission` approval pathway.
- Skip new tests for registry/factory scaffolding — the existing `provider_registry_test.go` pattern already proves definitions load.

## Risks / Trade-offs

- **[Risk]** ACP protocol is at `protocolVersion: 1` and still evolving (e.g., session capabilities, Streamable HTTP transport are in discussion upstream).
  **Mitigation:** Pin to `protocolVersion: 1`, fail loudly on mismatch during `initialize`, and decode unknown `session/update` variants into a `Raw` fallback so the client stays resilient to additive changes. A future bump will be a one-file update in the ACP client package.

- **[Risk]** Session resume does not work across daemon or Hermes subprocess restart.
  **Mitigation:** Document the limitation in the Hermes provider definition and in user-facing session-ended UX; do not advertise resume for Hermes the way Codex does. If/when Hermes implements on-disk session state in the ACP adapter, revisit by adding a `session/load`-by-disk-session-ID path.

- **[Risk]** Misadvertised capabilities cause silent feature breakage — e.g., claiming `SupportsApprovals` while a Hermes release regresses `session/request_permission`.
  **Mitigation:** The adapter treats an unknown or missing `session/request_permission` gracefully (turn proceeds without approval rather than hanging). Capability flags reflect the protocol-plus-implementation contract as of this change; if the contract changes, we downgrade the flag here rather than patching around it elsewhere.

- **[Risk]** ACP client package grows too large to review alongside the Hermes provider, blowing the one-PR guideline.
  **Mitigation:** If, during implementation, the `acp` package exceeds ~1200 LOC with tests, split the change into two sequential changes: "introduce ACP client" (package + tests) followed by "add Hermes provider on top of it" (provider + adapter + config + registry). Task 1.0 is explicitly a checkpoint for making this call.

- **[Trade-off]** Declining to advertise `fs.*`/`terminal` capabilities gives up the ability to route Hermes' file/terminal operations through archon-side interception (audit logging, sandboxing). This is consistent with how archon hosts Codex today (Codex owns its own sandboxing) and keeps the scope of this change contained.

## Migration Plan

- No data migration: this is a new provider. No existing sessions or configs need to change.
- Rollout: ship behind no flag. `hermes` is only selectable if the user has the `hermes` binary installed and on `$PATH` (same resolution pattern as `codex`/`claude`).
- Rollback: revert the PR. The change only adds files and registry entries; removing them restores the prior state with no database or config cleanup.

## Open Questions

- Does Hermes' ACP server currently surface `session/set_mode` or mode-updates? The spec supports it; if Hermes does too, we can wire it into the transcript adapter as a read-only mode indicator. If not, skip. Resolve by running `hermes acp` with a test harness during implementation (task 2.x) and inspecting `agentCapabilities` on the initialize response.
- Does `session/load` actually succeed within a live subprocess for a session created earlier in the same subprocess? Hermes' internals doc implies yes (the SessionManager is live for the process lifetime); confirm in the framing test suite rather than assuming.
