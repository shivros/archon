# Unified Transcript Boundary: Phase 6 Cleanup

## Scope Completed

Phase 6 cleanup is now fully transcript-first:

- Recents completion watching uses `/transcript/stream` SSE.
- UI transcript delivery uses only snapshot + transcript stream.
- Legacy provider transcript routes were removed from daemon and client internals.

## Canonical Contract

Transcript delivery is defined by:

- `GET /v1/sessions/:id/transcript`
- `GET /v1/sessions/:id/transcript/stream?follow=1`

Provider-specific ingestion remains an internal daemon concern behind transcript adapters.

## Breaking Change

The legacy transcript-equivalent SSE routes are removed:

- `GET /v1/sessions/:id/events?follow=1`
- `GET /v1/sessions/:id/items?follow=1`
