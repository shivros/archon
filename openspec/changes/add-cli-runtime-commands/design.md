## Context

These commands sit on the bootstrap path for almost every Archon workflow. `config` is read-only but has nuanced projection behavior. `daemon` multiplexes start and stop actions behind flags. `ui` must coordinate daemon readiness and version compatibility before handing control to Bubble Tea.

## Goals / Non-Goals

**Goals:**
- Specify the current user-visible contract for config inspection, daemon control, and UI launch.
- Preserve the existing output shapes used by docs and tests.
- Keep the requirements focused on command semantics, not implementation internals.

**Non-Goals:**
- Replacing config files with an interactive config editor.
- Reworking daemon process management.
- Specifying the full UI feature set after launch.

## Decisions

- Use one capability per command family because each command has its own user contract and test matrix.
- Treat `archon config` as a read-only inspection surface:
  - default output is JSON
  - TOML is an alternate rendering
  - `--default` bypasses malformed user files
  - `--scope` changes both what is loaded and the top-level payload shape
- Specify `archon daemon` in terms of flag precedence rather than the current helper implementation so future internals can change without breaking users.
- Specify `archon ui` around readiness checks:
  - default path verifies daemon version compatibility
  - `--restart-daemon` passes restart intent into the version check
  - `--ignore-daemon-mismatch` bypasses version gating but still requires a reachable daemon

## Risks / Trade-offs

- **`config` has several payload shapes depending on scope** -> The spec calls out the special-case shapes that downstream tooling and docs already rely on.
- **Daemon stop behavior depends on OS/process details** -> The spec focuses on observable CLI semantics rather than the process-signaling implementation.
- **`ui` launch is a thin wrapper around the TUI** -> The spec stops at the point the UI is launched and does not try to freeze internal Bubble Tea behavior.
