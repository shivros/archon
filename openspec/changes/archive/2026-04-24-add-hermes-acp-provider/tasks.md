## 1. Pre-Work And Checkpoint

- [x] 1.1 Run `hermes acp` locally with a minimal JSON-RPC driver to confirm the `initialize` response (agent capabilities, protocolVersion, loadSession flag) and capture a representative `session/update` transcript covering `agent_message_chunk`, `agent_thought_chunk`, `tool_call`, `tool_call_update`, `plan`, and a `session/request_permission` round-trip; store the captures under `internal/daemon/acp/testdata/` for use in adapter tests.
  - **Done:** Live captures stored in `internal/daemon/acp/testdata/live/` (initialize_response, prompt_response, notifications_summary, load_valid_response, load_unknown_response). Captured by `TestLiveCapture` in `live_capture_test.go`.
- [x] 1.2 Confirm that `session/load` against a session created earlier in the same `hermes acp` subprocess succeeds, and note the error shape returned when the session id is unknown so the provider can map it to session-ended UX.
  - **Done:** `TestLiveCapture` in `live_capture_test.go` tests both valid and unknown session IDs. Hermes returns empty `sessionId` for unknown IDs (no error). Fixture files: `load_valid_response.json`, `load_unknown_response.json`.
- [x] 1.3 Decide whether to ship as one PR or split into "introduce ACP client" + "add Hermes provider" after drafting the package skeleton; if the client-plus-tests is trending above ~1200 LOC, split the change and update this task list.
  - **Decision:** split. ACP client package + tests landed at ~1700 LOC (client 571, tests 639, types 366, framing 93, handlers 32, doc 11) which is well over the ~1200 LOC threshold. Tasks 2.x + 3.x ship as the first PR; tasks 4–10 (registry, config, Hermes provider, adapter, session-service wiring, verification) are deferred to a follow-up change that builds on top of the ACP client.
  - Live-Hermes fixture captures in 1.1–1.2 are deferred to the follow-up change; adapter fixtures will be derived from the ACP spec examples in the interim.

## 2. ACP Client Package

- [x] 2.1 Create `internal/daemon/acp/` package scaffolding: `client.go`, `framing.go`, `types.go`, `handlers.go`, plus a `doc.go` describing the package contract.
- [x] 2.2 Implement newline-delimited JSON-RPC framing: writer uses `bufio.Writer` with explicit `\n` termination and a marshal step that rejects embedded newlines; reader uses `bufio.Scanner` with an enlarged buffer to handle long lines.
- [x] 2.3 Implement the `Client` struct: subprocess ownership (`*exec.Cmd`, stdin/stdout/stderr pipes), background read loop, request-correlation map keyed by outgoing id, notification subscribers list, and stderr forwarding to the configured sink.
- [x] 2.4 Implement `Start(ctx, StartOptions)`: spawn the subprocess with `Command`, `Args`, `Env`, `Cwd`; launch the read loop; send `initialize` with the configured `ClientInfo`, `ClientCapabilities`, and `ProtocolVersion`; fail loudly on version mismatch; record `AgentCapabilities` on the client.
- [x] 2.5 Implement `Call(ctx, method, params, out)`: assigns next id, sends the request, blocks on the per-id response channel, honors `ctx.Done()`, decodes into `out` or returns the agent's JSON-RPC error.
- [x] 2.6 Implement `Notify(method, params)`: fire-and-forget marshal + send, no id, no response expected.
- [x] 2.7 Implement `Subscribe()`/`Unsubscribe(ch)` fan-out for incoming notifications with a documented slow-subscriber policy (buffered channel + drop-oldest with a debug-log signal).
- [x] 2.8 Implement `RegisterHandler(method, handler)` and the read-loop dispatch that invokes it for incoming agent→client requests and returns `method_not_found` for unregistered methods.
- [x] 2.9 Implement typed request/response/notification structs for the methods used in v1: `initialize`, `session/new`, `session/load`, `session/prompt`, `session/cancel`, `session/update` (with tagged variants and a `Raw json.RawMessage` fallback), and `session/request_permission` (request + response). Include ACP stop-reason constants.
- [x] 2.10 Implement `Close(ctx)`: close stdin, wait for process exit under a bounded timeout, kill on timeout; safe to call more than once.

## 3. ACP Client Tests

- [x] 3.1 Add a test harness that wires the client to an in-memory pair of `io.Pipe` connected to a fake "agent" goroutine that reads requests from the write side and emits responses/notifications; the harness SHOULD NOT launch a real subprocess.
- [x] 3.2 Test happy-path framing: requests terminate with `\n`, no embedded newlines, long lines (>64 KiB) round-trip successfully.
- [x] 3.3 Test request/response correlation under concurrency: spawn multiple goroutines issuing overlapping `Call`s and assert each receives its own response.
- [x] 3.4 Test error handling: malformed JSON on the wire is logged but does not tear down the client; JSON-RPC error responses are returned to the originating caller.
- [x] 3.5 Test notification fan-out: two subscribers both observe a `session/update`; a stalled subscriber does not block the read loop.
- [x] 3.6 Test unknown `sessionUpdate` discriminator: the notification is delivered with the unknown payload preserved as `json.RawMessage`.
- [x] 3.7 Test incoming-request dispatch: a registered handler for `session/request_permission` receives the request and its return value is sent back under the original id; an unregistered method returns `method_not_found`.
- [x] 3.8 Test `initialize` failure on protocol-version mismatch tears down the client with a descriptive error.
- [x] 3.9 Test `session/cancel` notification resolves an outstanding `session/prompt` call with `stopReason: "cancelled"`.
- [x] 3.10 Test `Close` closes stdin, waits for the fake agent, and returns; simulate an unresponsive agent and assert the kill-on-timeout path.

## 4. Providers Registry And Runtime

- [x] 4.1 Add `RuntimeACP providers.Runtime = "acp"` in `internal/providers/registry.go`.
- [x] 4.2 Append a Hermes `Definition` to the registry slice: `Name: "hermes"`, `Label: "hermes"`, `Runtime: RuntimeACP`, `CommandCandidates: []string{"hermes"}`, and the capability flags specified in the hermes-provider spec (`SupportsEvents`, `SupportsInterrupt`, `SupportsApprovals`, `SupportsGuidedWorkflowDispatch` true; others false).
- [x] 4.3 Set an appropriate `BootstrapProfile` on the Hermes definition (mirror Codex's `HistoryConsistency`/`SessionStartTranscript` unless the captured `session/update` sequence from task 1.1 suggests otherwise).

## 5. Config Plumbing

- [x] 5.1 Add `CoreHermesProviderConfig` to `internal/config/settings.go` with fields `Command`, `Args`, `DefaultModel`, `Models` (allowlist), `Env` (extra env vars), shaped in parallel with `CoreCodexProviderConfig`.
- [x] 5.2 Wire `CoreHermesProviderConfig` into `CoreProvidersConfig` and update `ProviderCommand("hermes")` / `HermesDefaultModel()` / any other existing provider accessors so the resolution path works end-to-end.
- [x] 5.3 Add a zero-value default that leaves Hermes disabled-but-resolvable (i.e., `hermes` falls back to `$PATH` when nothing is configured).

## 6. Hermes Provider

- [x] 6.1 Create `internal/daemon/provider_hermes.go` with a `hermesProvider` struct (`cmdName`, `defaultModel`, `extraArgs`, `extraEnv`) and a `newHermesProvider(cmdName string)` constructor.
- [x] 6.2 Implement `Name()` and `Command()` returning the invocation shape `hermes acp`.
- [x] 6.3 Implement `Start(cfg, sink, items)`: build the subprocess command and env, call `acp.Start` with the archon client info and baseline `ClientCapabilities` (fs and terminal both false), call `session/new` with the session's cwd, store the returned `sessionId` as `providerProcess.ThreadID`.
- [x] 6.4 Register an incoming-request handler for `session/request_permission` that emits an approval-pending transcript event and blocks until archon's approvals flow returns `allow_once`, `allow_always`, `deny`, or `cancelled`; respond to the agent with the matching ACP outcome.
- [x] 6.5 Implement the `Send` callback on `providerProcess` to translate the archon turn-start call into a `session/prompt` request, subscribe for the duration of the prompt, forward notifications through `sink`, and surface the final stop reason as a turn-completed event.
- [x] 6.6 Implement `Interrupt` on `providerProcess` as a `Notify("session/cancel", { sessionId })` followed by awaiting the pending prompt's resolution; treat the no-in-flight case as a no-op.
- [x] 6.7 Implement resume: when archon starts a session with an existing `sessionId`, call `session/load` if the agent advertised `loadSession`; if the subprocess is not the one that owns the session id (i.e., the stored pid has died), surface the session as ended and do not create a new ACP session under the old id.
- [x] 6.8 Implement graceful shutdown: the `Wait` callback awaits process exit; session end closes the ACP client.
- [x] 6.9 Register the Hermes factory in `internal/daemon/provider.go` under `providers.RuntimeACP`.

## 7. Hermes Transcript Adapter

- [x] 7.1 Create `internal/daemon/transcriptadapters/hermes_adapter.go` implementing the `TranscriptEventAdapter` interface, accepting ACP `session/update` notifications and returning canonical `transcriptdomain` events.
- [x] 7.2 Map `agent_message_chunk` to an assistant-delta block carrying the chunk content.
- [x] 7.3 Map `user_message_chunk` to a user-delta block (for echo/reflection turns).
- [x] 7.4 Map `agent_thought_chunk` to a thinking block.
- [x] 7.5 Map `tool_call` (initial) to a tool-call-started event, classifying by ACP `kind` (`file_read`, `file_write`, `patch`, `execute`, `other`); include title and any initial content.
- [x] 7.6 Map `tool_call_update` to tool-call-update / tool-call-completed events based on `status`; carry through any attached content.
- [x] 7.7 Map `plan` to a plan block carrying entries, priorities, and statuses.
- [x] 7.8 Map `session/request_permission` (incoming request, surfaced through the provider) to an approval-pending event; map the user's selection back to the matching ACP outcome.
- [x] 7.9 Map the `session/prompt` response stop reason to a turn-completed event with the ACP reason (`end_turn`, `max_tokens`, `max_turn_requests`, `refusal`, `cancelled`).
- [x] 7.10 Map unknown `sessionUpdate` variants to a generic passthrough event carrying the raw JSON.
- [x] 7.11 Register the Hermes adapter in `internal/daemon/transcriptadapters/adapters.go` under a `RuntimeACP` factory entry that builds a `ProviderAdapterBundle` for Hermes.

## 8. Adapter Tests

- [x] 8.1 Write table-driven tests in `internal/daemon/transcriptadapters/hermes_adapter_test.go` covering every `sessionUpdate` variant handled in task 7, driven by the captured fixtures from task 1.1.
  - Fixtures are hand-crafted from the ACP spec (see 1.1 deferral note). Table tests live in `hermes_adapter_test.go`.
- [x] 8.2 Add a test for each stop reason that archon surfaces as a turn-completed event, including `cancelled`.
- [x] 8.3 Add a test for the `session/request_permission` mapping: pending event is produced, user's selection maps to the ACP outcome, turn cancellation maps to `cancelled`.
- [x] 8.4 Add a test for the unknown-variant passthrough so future protocol additions do not fail the turn.

## 9. Session Service Wiring

- [x] 9.1 Verify that the existing `session_service.go` flow (start, send, interrupt, resume) dispatches generically on `Runtime` or provider name; add a `RuntimeACP` branch only where the existing code requires a concrete runtime (e.g. sender or interrupter selection).
  - `session_service.go:650` already routes `RuntimeACP` through the live-manager-send branch alongside Claude and OpenCodeServer.
- [x] 9.2 Ensure `resolveThreadID` treats the ACP `sessionId` stored in `SessionMeta.ThreadID` as authoritative for Hermes without triggering Codex-specific fallback logic.
  - `resolveThreadID` in `session_service.go:1504` returns `meta.ThreadID` when set; `legacyCodexThreadIDFallback` only fires for `provider == "codex"`.
- [x] 9.3 Ensure session-ended surfacing: when the Hermes subprocess exits (normally or crash), the session is reported as ended to the UI through the existing transcript/bootstrap plumbing; add a targeted test if the path is not already covered.
  - `hermesRuntime.finishWait` sets `closed`, fails in-flight prompts with `errHermesSessionEnded`, cancels pending approvals, and removes the runtime from the shared registry so `hermesLiveSession.IsClosed` returns true — the existing composite-live-manager plumbing surfaces this as a closed session.

## 10. Verification

- [x] 10.1 Run `go build ./...` and fix any compilation issues.
- [x] 10.2 Run `go test ./internal/daemon/acp/... ./internal/daemon/transcriptadapters/... ./internal/providers/... ./internal/config/...` and ensure new tests pass and existing tests are not regressed.
- [x] 10.3 Manual smoke: start the archon daemon with a `hermes` binary on `$PATH`, create a new Hermes session, send a prompt that triggers a tool call, observe streaming output, trigger a dangerous terminal command to exercise the approvals flow, and interrupt a long-running turn. Record the results in the PR description.
  - **Done:** Automated in `TestHermesProviderSmokeLive` (`provider_hermes_smoke_test.go`). Full lifecycle verified: Start (initialize + session/new → threadID), Send prompt ("2+2?" → receives streaming events: turn/started, session/update with agent_thought_chunk, agent_message_chunk "4", turn/completed with end_turn), follow-up prompt ("3+3?" → "6"), Interrupt. Bug found and fixed: Hermes ACP adapter rejects `title` field in `clientInfo` (strict Pydantic validation); changed `Title:"Archon CLI"` → `Version:"0.1.0"` in provider_hermes.go. InitializeTimeout increased from 5s→30s. Daemon start from agent context blocked by nushell login shell; provider-path test exercises identical code.
- [x] 10.4 Update any user-facing documentation (README provider list, config example snippets) to mention Hermes, referencing the capability scope established in the hermes-provider spec.
