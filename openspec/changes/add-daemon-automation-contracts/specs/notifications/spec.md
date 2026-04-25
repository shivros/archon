## ADDED Requirements

### Requirement: Notification settings SHALL resolve with layered override precedence
Archon SHALL resolve daemon-side notification settings by starting from global notification defaults and then applying optional worktree overrides followed by optional session overrides. Session overrides SHALL take precedence over worktree overrides, which SHALL take precedence over global defaults.

#### Scenario: Session overrides beat worktree overrides
- **WHEN** both a worktree and a session define notification overrides for the same session
- **THEN** Archon MUST resolve the effective notification settings using the session override values in preference to the worktree values

### Requirement: Notification dispatch SHALL support explicit methods and `auto` fallback
Archon SHALL support configured notification methods including `dunstify`, `notify-send`, `bell`, and `auto`. When `auto` is selected, Archon SHALL attempt the built-in notification sinks in fallback order until one succeeds or no sink remains.

#### Scenario: `auto` falls back across built-in sinks
- **WHEN** notifications are enabled with method `auto`
- **AND** the first built-in sink is unavailable or fails
- **THEN** Archon MUST try the next built-in sink
- **AND** the dispatcher MUST stop once one sink succeeds

#### Scenario: Unknown notification method is rejected
- **WHEN** effective notification settings resolve to an unknown method
- **THEN** notification dispatch MUST fail with an explicit error rather than silently dropping the event

### Requirement: `script_commands` SHALL receive the event payload on stdin plus Archon metadata in environment variables
When `script_commands` are configured, Archon SHALL execute each configured script command with the notification event JSON on stdin. The process environment SHALL include the notification metadata variables documented in the README.

#### Scenario: Script command receives event JSON payload
- **WHEN** notification dispatch executes a configured script command
- **THEN** the command's stdin MUST contain the notification event JSON payload
- **AND** the payload MUST include the session id and trigger values for that event

#### Scenario: Script command receives Archon notification metadata in the environment
- **WHEN** Archon launches a configured notification script command
- **THEN** the process environment MUST include the documented `ARCHON_*` notification metadata variables

### Requirement: Duplicate notifications SHALL be suppressed within the dedupe window
Archon SHALL suppress duplicate notification deliveries for the same logical event within the configured dedupe window so noisy status updates do not produce repeated alerts.

#### Scenario: Duplicate turn-completed event inside the dedupe window is suppressed
- **WHEN** the same logical notification event is published twice within the configured dedupe window
- **THEN** the first event MUST be delivered
- **AND** the later duplicate MUST be suppressed

#### Scenario: Event outside the dedupe window is delivered again
- **WHEN** an equivalent notification event arrives after the dedupe window has elapsed
- **THEN** Archon MUST allow it to be delivered again
