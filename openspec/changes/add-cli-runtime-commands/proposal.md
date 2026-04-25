## Why

`archon config`, `archon daemon`, and `archon ui` are part of the repo's documented day-one workflow, but none of them have spec coverage yet. That leaves the CLI/runtime bootstrap path vulnerable to accidental behavior drift even though users depend on it to inspect config, control the daemon, and enter the TUI.

## What Changes

- Add a CLI contract for `archon config`, including output formats, default-vs-effective rendering, and scoped projections.
- Add a CLI contract for `archon daemon`, including foreground/background start, stop, and forced restart behavior.
- Add a CLI contract for `archon ui`, including daemon-version checks, restart behavior, and the daemon-mismatch escape hatch.
- Lock shared error-handling expectations for these runtime-management commands.

## Capabilities

### New Capabilities
- `cli-config-inspection`: Defines how `archon config` renders effective or default configuration in JSON or TOML, including scoped views.
- `cli-daemon-control`: Defines how `archon daemon` starts, stops, and force-restarts the local daemon.
- `cli-ui-launch`: Defines how `archon ui` validates daemon readiness before launching the TUI.

### Modified Capabilities

## Impact

- **Affected code:**
  - `cmd/archon/command_config.go`
  - `cmd/archon/command_daemon.go`
  - `cmd/archon/command_ui.go`
  - `cmd/archon/commands.go`
  - `cmd/archon/commands_test.go`
  - `README.md`
- **Affected behavior:** No new end-user features are intended; this change formalizes the current runtime-management contract.
- **Dependencies:** Existing config loaders, daemon admin client helpers, and TUI launcher.
- **Out of scope for this change:**
  - Changing config file formats or schema
  - Redesigning daemon health/version negotiation
  - Specifying all TUI behavior after launch
