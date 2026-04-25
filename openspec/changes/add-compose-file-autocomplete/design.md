## Context

Compose autocomplete spans the UI, typed client, daemon handlers, and provider adapters. The user sees a provider-agnostic picker, but the implementation fans out into provider-specific search runtimes. The spec needs to freeze the cross-layer contract without overcommitting to any one adapter's internals.

## Goals / Non-Goals

**Goals:**
- Specify the user-visible compose autocomplete behavior.
- Specify the daemon/client file-search route contract that powers it.
- Preserve the current V1 choice of textual mention insertion.

**Non-Goals:**
- Adding support for providers that currently lack file search.
- Defining a future structured-mention payload format.
- Reworking the UI picker design.

## Decisions

- Split the change into two capabilities:
  - one for the compose-side UX contract
  - one for the daemon/client file-search transport contract
- Treat provider support as an explicit compatibility boundary:
  - supported providers open the picker and perform normalized search
  - unsupported providers leave `@` as plain text and surface a specific unsupported error through the API layer
- Keep V1 insertion textual (`@path/to/file`) even though the daemon uses structured search sessions internally. This matches the current app architecture and avoids provider lock-in.

## Risks / Trade-offs

- **The feature spans many layers** -> The spec intentionally focuses on shared behavior and route semantics rather than locking each internal controller implementation.
- **Provider support differs today** -> The requirements describe graceful degradation so unsupported runtimes do not look broken.
- **A future structured-mention design may replace textual insertion** -> That should land as a separate intentional change because it would alter both UI and transport contracts.
