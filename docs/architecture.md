# Architecture

This note documents the current flow so refactors can preserve behavior.

## Request/Response Flow

```
UI (internal/app) -> typed HTTP/SSE client (internal/client)
                   -> daemon API (internal/daemon)
                   -> provider/session runtime (codex, claude, custom)
```

1. `cmd/archon/main.go` starts either the daemon or the Bubble Tea UI.
2. The UI `Model` in `internal/app/model.go` coordinates modes, selection, rendering, polling, and stream consumption.
3. The UI talks to the daemon through interfaces in `internal/app/api.go`, backed by `internal/client.Client`.
4. The client uses REST endpoints under `/v1/...` and SSE endpoints for live streams:
   - `/v1/sessions/:id/tail?follow=1` for log stream chunks
   - `/v1/sessions/:id/events?follow=1` for codex events
   - `/v1/sessions/:id/items?follow=1` for item-based providers
5. `internal/daemon/api.go` handles HTTP transport/routing and delegates to services (`SessionService`, workspace/state services).
6. `SessionService` and `SessionManager` orchestrate provider adapters:
   - codex provider (`provider_codex.go`)
   - claude provider (`provider_claude.go`)
   - generic process provider (`provider_exec.go`)

## Streaming and Persistence

- Streaming state in UI is consumed via:
  - `StreamController` (log chunks),
  - `CodexStreamController` (event stream + approvals),
  - `ItemStreamController` (item stream providers).
- Persistent app/session metadata is stored by daemon-backed stores in `internal/store` and retrieved by the UI through snapshot calls (`sessions`, `history`, `approvals`, app state).
- UI keeps a transcript cache keyed by sidebar selection so switching sessions is fast while still reconciling with history snapshots.

## Status and Toast Policy

- UI status/toast behavior is centralized in `internal/app/model_status_policy.go`.
- Event categories and toast severity rules are documented in `docs/status-policy-matrix.md`.
- New status patterns should extend the policy table instead of writing direct `m.status = ...` assignments.

## Phase 0 Baseline Contract

Phase 0 must keep behavior stable for:

- streaming updates and close states,
- compose/send local state transitions,
- session selection load/reset behavior,
- approval visibility in both polling and event-driven paths.
