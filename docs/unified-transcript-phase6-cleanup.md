# Unified Transcript Boundary: Phase 6 Cleanup

## Scope Completed

Phase 6 cleanup focused on reducing internal legacy transcript paths while preserving external compatibility:

- Migrated recents turn-completion watching from provider-specific `/events` SSE to unified `/transcript/stream` SSE.
- Removed unused UI command/controller scaffolding that opened provider-specific event/items streams.
- Kept legacy daemon routes (`/events`, `/items`) in place as compatibility shims.

## Compatibility Retained Intentionally

The following routes remain intentionally:

- `GET /v1/sessions/:id/events?follow=1`
- `GET /v1/sessions/:id/items?follow=1`

Reason:

- No explicit approval to remove public compatibility paths.
- Existing integration tests still exercise these routes.

## Operational Rollout Guidance

- Internal UI path is now unified transcript-first for transcript delivery.
- External clients can migrate to:
  - `GET /v1/sessions/:id/transcript`
  - `GET /v1/sessions/:id/transcript/stream?follow=1`
- Legacy route removal should only happen after:
  1. usage evidence confirms no active clients, or
  2. explicit approval is granted for breaking removal.
