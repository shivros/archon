## Context

Provider badges are a small but visible part of the session sidebar. The implementation already has stable defaults, accepts per-provider overrides from app state, normalizes override keys, and falls back sensibly for unknown providers. This change needs to lock that contract without turning it into a larger theming proposal.

## Goals / Non-Goals

**Goals:**
- Specify the default provider badge behavior visible in the sidebar.
- Specify how `provider_badges` overrides from `state.json` are normalized and applied.
- Specify app-state persistence for those overrides.

**Non-Goals:**
- Redesigning the sidebar.
- Adding extra badge metadata beyond `prefix` and `color`.
- Defining a general theming API for all UI tokens.

## Decisions

- Keep the entire behavior under one capability because default rendering, override resolution, and persistence are one cohesive user-facing contract.
- Specify override behavior as selective:
  - non-empty override `prefix` replaces the default prefix
  - non-empty override `color` replaces the default color
  - omitted or blank fields leave the corresponding default intact
- Preserve current normalization semantics:
  - provider keys are normalized before lookup
  - blank or invalid provider keys are ignored
  - unknown providers still get a derived fallback badge

## Risks / Trade-offs

- **This is a small feature surface** -> Keeping it in one compact change avoids unnecessary fragmentation.
- **Defaults can vary by theme internals** -> The spec locks the presence of provider-specific defaults and fallback behavior, while leaving room for the theme system to supply the actual default colors.
- **Users may expect arbitrary badge styling later** -> Additional styling fields can be added in a future deliberate change without weakening the current prefix/color contract.
