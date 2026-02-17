# archon
Manage AI CLI sessions across repos and AI vendors.

## Development
This repo uses `prek` (a Rust pre-commit runner) with a standard `.pre-commit-config.yaml`.

Install git hooks:
```bash
prek install
```

Run hooks manually:
```bash
prek run --all-files
```

## Session Provider Badges
Session rows in the TUI sidebar show provider badges (for example `[CDX]`, `[CLD]`, `[OPN]`). You can override badge prefix/color per provider by setting `provider_badges` in `~/.archon/state.json`:

```json
{
  "provider_badges": {
    "codex": { "prefix": "[GPT]", "color": "15" },
    "claude": { "prefix": "[CLD]", "color": "208" },
    "opencode": { "prefix": "[OPN]", "color": "39" },
    "kilocode": { "prefix": "[KLO]", "color": "226" }
  }
}
```

`color` accepts Lip Gloss-compatible terminal colors (ANSI index like `"208"` or hex like `"#ff8a3d"`).

## Configuration Files
Archon now separates core/daemon config, UI config, and UI keybindings:

- `~/.archon/config.toml` (core daemon/client config)
- `~/.archon/ui.toml` (UI config)
- `~/.archon/keybindings.json` (UI hotkey overrides)
- `~/.archon/workflow_templates.json` (guided workflow templates + per-step prompts)

Configuration is file-based:

- `config.toml` controls daemon/core behavior (daemon address, provider defaults, logging/debug settings).
- `ui.toml` controls UI-level settings.
- `keybindings.json` overrides UI hotkeys.
- `workflow_templates.json` stores user-defined guided workflow templates and prompts.

Example `config.toml`:

```toml
[daemon]
address = "127.0.0.1:7777"

[logging]
level = "info" # debug | info | warn | error

[debug]
stream_debug = false

[notifications]
enabled = true
triggers = ["turn.completed", "session.failed", "session.killed", "session.exited"]
methods = ["auto"] # auto | notify-send | dunstify | bell
script_commands = [] # shell commands fed JSON payload via stdin
script_timeout_seconds = 10
dedupe_window_seconds = 5

[guided_workflows]
enabled = false
auto_start = false
checkpoint_style = "confidence_weighted"
mode = "guarded_autopilot"

[guided_workflows.policy]
confidence_threshold = 0.70
pause_threshold = 0.60
high_blast_radius_file_count = 20

[guided_workflows.policy.hard_gates]
ambiguity_blocker = true
confidence_below_threshold = false
high_blast_radius = false
sensitive_files = true
pre_commit_approval = false
failing_checks = true

[guided_workflows.policy.conditional_gates]
ambiguity_blocker = true
confidence_below_threshold = true
high_blast_radius = true
sensitive_files = false
pre_commit_approval = false
failing_checks = true

[guided_workflows.rollout]
telemetry_enabled = true
max_active_runs = 3
automation_enabled = false
allow_quality_checks = false
allow_commit = false
require_commit_approval = true
max_retry_attempts = 2

[providers.codex]
command = "codex"
default_model = "gpt-5.1-codex"
models = ["gpt-5.1-codex", "gpt-5.2-codex", "gpt-5.3-codex", "gpt-5.1-codex-max"]
approval_policy = "on-request"
sandbox_policy = "workspace-write"
network_access = false

[providers.claude]
command = "claude"
default_model = "sonnet"
models = ["sonnet", "opus"]
include_partial = false

[providers.opencode]
base_url = "http://127.0.0.1:4096"
token = ""
token_env = "OPENCODE_TOKEN"
username = "opencode"
timeout_seconds = 90

[providers.kilocode]
base_url = "http://127.0.0.1:4097"
token = ""
token_env = "KILOCODE_TOKEN"
username = "kilocode"
timeout_seconds = 90

[providers.gemini]
command = "gemini"
```

OpenCode/Kilo prompt requests are long-running by design; Archon enforces a runtime minimum of `90` seconds for these providers.

### Notifications

Archon supports daemon-side notifications with layered overrides:

- global defaults from `~/.archon/config.toml` `[notifications]`
- per-worktree overrides (`worktree.notification_overrides`)
- per-session overrides (`session_meta.notification_overrides`)

Precedence is: `session override` > `worktree override` > `global defaults`.

`script_commands` are executed with the notification event JSON on stdin and these env vars:

- `ARCHON_EVENT`
- `ARCHON_SESSION_ID`
- `ARCHON_WORKSPACE_ID`
- `ARCHON_WORKTREE_ID`
- `ARCHON_PROVIDER`
- `ARCHON_STATUS`
- `ARCHON_TURN_ID`
- `ARCHON_CWD`
- `ARCHON_NOTIFICATION_AT`

### Guided Workflows

Enable guided workflows in `~/.archon/config.toml`:

- set `[guided_workflows].enabled = true`
- keep `auto_start = false` (default) to require explicit user start from task/worktree context
- tune `[guided_workflows.policy]` and `[guided_workflows.rollout]` guardrails as needed

When enabled, daemon exposes guided workflow lifecycle endpoints:

- `POST /v1/workflow-runs`
- `POST /v1/workflow-runs/:id/start`
- `POST /v1/workflow-runs/:id/pause`
- `POST /v1/workflow-runs/:id/resume`
- `POST /v1/workflow-runs/:id/decision`
- `GET /v1/workflow-runs/:id`
- `GET /v1/workflow-runs/:id/timeline`
- `GET /v1/workflow-runs/metrics`
- `POST /v1/workflow-runs/metrics/reset`

Telemetry snapshots are persisted in daemon app state, so aggregate workflow metrics survive daemon restarts.
Use `POST /v1/workflow-runs/metrics/reset` to reset aggregate counters for a fresh rollout/measurement window.

Manual start flow:

- from workspace/worktree/session context in the TUI, choose `Start Guided Workflow`
- configure run setup (template + policy sensitivity)
- launch run and monitor the timeline/decision inbox surfaces

Checkpoint behavior:

- policy emits explicit decisions (`continue` or `pause`) with reasons/severity/tier metadata
- pauses produce actionable decision-needed notifications (`approve_continue`, `request_revision`, `pause_run`)
- decision payload includes `reason`, `confidence`, `risk_summary`, and `recommended_action`

`POST /v1/workflow-runs/:id/decision` accepts:

- `action`: `approve_continue` | `request_revision` | `pause_run`
- `decision_id` (optional)
- `note` (optional)

When turn-completed events progress a run and policy decides `pause`, Archon emits a decision-needed notification through the existing notification pipeline (`turn.completed` trigger, `status=decision_needed`) with structured payload fields including:

- `reason`
- `confidence`
- `risk_summary`
- `recommended_action`
- `actions` (available decision actions)

Troubleshooting:

- `guided workflows are disabled`: verify `[guided_workflows].enabled = true` and restart daemon
- `workflow active run limit exceeded`: raise `[guided_workflows.rollout].max_active_runs` or wait for active runs to finish
- repeated pause decisions: inspect `risk_summary` and `trigger_reasons` in decision notifications, then adjust policy thresholds/gates
- metrics not changing: confirm `[guided_workflows.rollout].telemetry_enabled = true` and query `GET /v1/workflow-runs/metrics`

Example `ui.toml`:

```toml
[keybindings]
path = "keybindings.json"

[chat]
timestamp_mode = "relative" # relative | iso

[sidebar]
expand_by_default = true
```

Example `keybindings.json` (VS Code-style array):

```json
[
  { "command": "ui.toggleSidebar", "key": "alt+b" },
  { "command": "ui.refresh", "key": "F5" }
]
```

You can inspect the resolved/effective config at any time:

```bash
archon config
```

`archon config` prints configuration to stdout. It does not generate or modify config files.

Useful options:

- `--default`: print built-in defaults (ignores local config files)
- `--format json|toml`: output format (default: `json`)
- `--scope core|ui|keybindings|all`: print one scope or multiple scopes (repeatable)

Examples:

```bash
archon config --scope core
archon config --scope ui --format toml
archon config --scope keybindings --default
archon config --scope core --scope keybindings --format json
```

Clipboard copy always tries the system clipboard first, then OSC52 as fallback.

## Keybinding Command IDs
The following command IDs are supported in `keybindings.json`:

- `ui.menu`
- `ui.quit`
- `ui.toggleSidebar`
- `ui.toggleNotesPanel`
- `ui.copySessionID`
- `ui.openSearch`
- `ui.viewportTop`
- `ui.viewportBottom`
- `ui.sectionPrev`
- `ui.sectionNext`
- `ui.searchPrev`
- `ui.searchNext`
- `ui.newSession`
- `ui.addWorkspace`
- `ui.addWorktree`
- `ui.compose`
- `ui.openNotes`
- `ui.openChat`
- `ui.refresh`
- `ui.killSession`
- `ui.interruptSession`
- `ui.dismissSession`
- `ui.undismissSession`
- `ui.toggleDismissed`
- `ui.toggleNotesWorkspace`
- `ui.toggleNotesWorktree`
- `ui.toggleNotesSession`
- `ui.toggleNotesAll`
- `ui.pauseFollow`
- `ui.toggleReasoning`
- `ui.toggleMessageSelect`
- `ui.composeClearInput`
- `ui.composeModel`
- `ui.composeReasoning`
- `ui.composeAccess`
- `ui.inputSubmit`
- `ui.inputNewline`
- `ui.inputWordLeft`
- `ui.inputWordRight`
- `ui.inputDeleteWordLeft`
- `ui.inputDeleteWordRight`
- `ui.inputSelectAll`
- `ui.inputUndo`
- `ui.inputRedo`
- `ui.approve`
- `ui.decline`
- `ui.notesNew`
