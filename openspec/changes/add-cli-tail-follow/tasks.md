## 1. Client Adapter Surface

- [x] 1.1 In `cmd/archon/client_adapter.go`, extend the `sessionClientFactory`-produced client interface with a `StreamTail(ctx context.Context, sessionID, streamName string) (<-chan types.LogEvent, func(), error)` method (matching `internal/client/sse.go`'s existing `TailStream` return signature).
- [x] 1.2 Implement the new method on `controlClientAdapter` so it delegates to the existing streaming tail helper `c.client.TailStream` in `internal/client/sse.go` without re-implementing SSE framing.
- [x] 1.3 Update the test fakes in `cmd/archon/commands_test.go` (`fakeCommandClient`) to satisfy the new interface method with a controllable channel-based stub.

## 2. CLI Flags And Follow Path

- [x] 2.1 In `cmd/archon/command_tail.go`, add a `--follow`/`-f` boolean flag and a `--stream` string flag defaulting to `combined`.
- [x] 2.2 Keep the existing `--lines` behavior unchanged.
- [x] 2.3 When `--follow` is NOT set, leave the current snapshot path intact (`TailItems` → JSON array).
- [x] 2.4 When `--follow` IS set, open the stream via the new `StreamTail` adapter method using a `context.Context` that is cancelable from a signal handler.

## 3. NDJSON Output And Flushing

- [x] 3.1 In follow mode, marshal each received item with `encoding/json.Marshal` (NOT `MarshalIndent`) to produce a single-line JSON document.
- [x] 3.2 Write the JSON followed by exactly one `\n` to `c.stdout`.
- [x] 3.3 If `c.stdout` implements a flusher (local `flusher` interface with `Flush() error`), flush after each line; otherwise rely on the runtime's default stdout flushing at process exit.

## 4. Backfill + Follow Composition

- [x] 4.1 When both `--lines` and `--follow` are set, call `TailItems` first and emit each returned item as NDJSON before opening the stream.
- [x] 4.2 After the backfill, open the stream and drop any initial event whose content semantically matches the last backfill item (using `jsonMapsEqual` for key-order-independent comparison) to avoid duplicates.
- [x] 4.3 Guarantee the transition writes no separator, heading, or blank line between backfill and live events — the output must remain uniform NDJSON.

## 5. Signal Handling And Exit Codes

- [x] 5.1 In follow mode, install a `signal.NotifyContext` handler for `SIGINT` and `SIGTERM`; on receipt cancel the stream context.
- [x] 5.2 Clean termination paths (signal received, daemon closed stream with session-ended semantic) return nil from `Run` so the process exits `0`.
- [x] 5.3 Error termination paths (network error, scan error, daemon-side stream error) return a non-nil error from `Run` so the process exits non-zero; the error message is a single line.
- [x] 5.4 Deregister the signal handler on `Run` return (via `defer stop()`) so that in-process test harnesses are not polluted.

## 6. Tests

- [x] 6.1 Snapshot path regression: assert `archon tail <id>` without `--follow` still emits the existing JSON array.
- [x] 6.2 Follow path happy case: pipe a handful of synthetic items through the fake adapter's channel and assert stdout receives valid NDJSON with one object per line.
- [x] 6.3 Follow path stream selector: assert `--stream stderr` is propagated into the adapter call.
- [x] 6.4 Follow path SIGINT: cancel the command's context (proxy for a received signal in tests) and assert `Run` returns nil with the partial stdout output intact.
- [x] 6.5 Follow path error: have the fake adapter close its channel with an error and assert `Run` returns a non-nil error with a single-line message.
- [x] 6.6 Backfill-then-follow: seed both a snapshot response and a stream channel, assert backfill items print before stream items, and assert no duplicate at the boundary.

## 7. Documentation

- [x] 7.1 Update the `archon tail` entry in `README.md`'s CLI reference to document `--follow`, `-f`, and `--stream`, calling out that follow-mode output is NDJSON.
- [x] 7.2 Update the flag usage strings in `command_tail.go` so `archon tail -h` reflects the new flags and the NDJSON output contract.

## 8. Verification

- [x] 8.1 Run `go build ./...` and confirm no regressions.
- [x] 8.2 Run `go test ./cmd/archon/...` and confirm new tests pass and existing tests are unchanged.
- [x] 8.3 Manual smoke: restarted daemon with new binary, confirmed `--follow` emits NDJSON backfill items, `--follow` with flags after session ID works (via `reorderFlagsBeforePositional`), and exit codes are correct. Note: full live-stream smoke requires a daemon-side SSE endpoint for an active session, which is a separate change.
