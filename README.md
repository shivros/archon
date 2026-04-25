# archon

Archon is a TUI-based session manager for AI coding agents. It lets you run, monitor, and orchestrate multiple AI CLI sessions across repositories and providers from a single terminal interface. Features include live event streaming, approval handling, session resume, guided multi-step workflows, and configurable notifications.

> **Alpha software.** Archon is under active development and changing rapidly. Feature support varies across providers and may break between releases. See the support grid below for current status.

![Archon UI](ui.png)

## Provider Support

Codex is the primary and most thoroughly tested provider. Support for other providers is in progress.

| Feature | Codex | Claude | OpenCode / Kilo Code | Hermes |
|---|:---:|:---:|:---:|:---:|
| **Model Selection** | Full | Partial | Full | - |
| **Reasoning Levels** | Full | - | - | - |
| **Access Levels** | Full | Full | - | - |
| **Live Events / Streaming** | Full | Partial | Partial | Partial |
| **Approvals** | Full | Partial | Partial | Partial |
| **Interrupt** | Full | Partial | Full | Partial |
| **Session Resume** | Full | Full | Full | Process-local only |
| **Compose File Autocomplete (`@...`)** | Full | - | Full | - |
| **Guided Workflows** | Full | Full | Partial | Partial |
| **Notifications** | Full | Partial | Partial | Partial |

Hermes speaks the Agent Client Protocol (ACP) over stdio. Archon launches it as `hermes acp` and streams tokens, tool calls, plans, and approval requests. File search is not supported (ACP has no dedicated verb) and resume is scoped to the live subprocess — once the Hermes process exits, the session is surfaced as ended.

**Full** = well-tested and reliable, **Partial** = works but incomplete or lightly tested, **-** = not supported.

Gemini and Custom providers have basic exec-only support and no feature parity yet.

## Compose File Autocomplete

Compose supports `@`-triggered file autocomplete through one provider-agnostic file-search interface.

- V1 inserts plain-text mentions such as `@pkg/main.go`.
- The UI stays provider-agnostic; provider-specific search behavior lives behind daemon/provider adapters.
- Current support is available for `codex`, `opencode`, and `kilocode`.
- Unsupported providers keep `@` as plain text and do not open the picker.

Implementation details vary by provider:

- Codex uses its app-server fuzzy file search support.
- OpenCode and Kilo Code use their server-side file search endpoints.
- Results are normalized before they reach the app layer so compose behavior stays consistent.

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

The hook set includes a guard against reintroducing deprecated guided-workflow contract fields in non-test Go code.

### Build & Release

Quick local build (output: `dist/archon`):

```bash
make build
./dist/archon version
```

Build metadata is injected via ldflags:

- `VERSION` (default `dev`)
- `COMMIT` (default current git short SHA when available)
- `BUILD_DATE` (default current UTC RFC3339 timestamp)

Maintainer operational guidance (CI scope, manual artifact builds, manual release publishing, and end-to-end runbook flow) is authoritative in [docs/maintainer-build-release-runbook.md](docs/maintainer-build-release-runbook.md).

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
Provider keys are normalized by trimming whitespace and lowercasing before lookup. You can set only
`prefix` or only `color`; omitted or blank fields keep the built-in default for that provider. Unknown
providers fall back to a derived three-character badge and the default fallback color.

## Configuration Files
Archon separates core/daemon config, UI config, and UI keybindings:

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

[cloud]
base_url = "https://app.archon.ai"
browser_base_url = "https://app.archon.ai" # optional: activation/browser host if different from the API host
client_id = "archon-cli"
timeout_seconds = 10

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

[guided_workflows.defaults]
provider = "codex" # codex | claude | opencode | kilocode (unsupported values fail explicitly)
model = "gpt-5.1-codex"
access = "on_request" # read_only | on_request | full_access
reasoning = "medium" # low | medium | high | extra_high
resolution_boundary = "balanced" # low | balanced | high

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

[title_generation]
provider = "" # "" disables feature, "openrouter" enables async AI title generation
model = "openrouter/auto"
timeout_seconds = 10

[title_generation.openrouter]
api_key = "" # optional; prefer api_key_env
api_key_env = "OPENROUTER_API_KEY"
base_url = "https://openrouter.ai/api/v1"

[providers.codex]
command = "codex"
default_model = "gpt-5.1-codex"
models = ["gpt-5.1-codex", "gpt-5.4-codex", "gpt-5.3-codex", "gpt-5.1-codex-max"]
approval_policy = "on-request"
sandbox_policy = "workspace-write"
network_access = false

[providers.claude]
command = "claude"
default_model = "sonnet"
models = ["sonnet", "opus"]
include_partial = false

[providers.hermes]
command = "hermes"
# args prepended before the `acp` subcommand, env vars appended to the subprocess environment
args = []
env = []
default_model = ""
models = []

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

### Starting Sessions

`archon start` creates a new session. It requires `--provider` and prints only the session id on stdout:

```bash
# Basic
archon start --provider codex --cwd /tmp/project

# With all options
archon start --provider codex --cwd /tmp/project --cmd codex --title "my session" --tag one --tag two --env A=B -- arg1 arg2

# Quick smoke test
archon start --provider codex --cwd /tmp -- "echo hello"
```

Flags:
- `--provider` (required): provider name (e.g. `codex`, `claude`)
- `--cwd`: working directory for the session
- `--cmd`: command override (custom provider)
- `--title`: session title
- `--tag` (repeatable): tags attached to the session
- `--env` (repeatable): environment variables in `KEY=VALUE` form
- Trailing positional arguments after `--` are forwarded as command args

### Listing Sessions

`archon ps` lists every known session. Default output is a human-readable tab-separated table:

```bash
archon ps
```

For programmatic consumers (scripts, LLM agents), pass `--json` to get the daemon's full session DTO as a JSON array — one document per invocation:

```bash
archon ps --json | jq '.[].id'
```

The JSON shape is the daemon's `types.Session` serialization and is considered part of the CLI contract: fields like `id`, `status`, `provider`, `pid`, and `title` are load-bearing for downstream automation.

### Session Details

`archon session <id>` returns the full `types.Session` for a single session. Default output is pretty-printed JSON — the same shape as each element in `ps --json`:

```bash
archon session abc123
archon session abc123 | jq .status
```

For interactive inspection, `--format human` prints a compact field-per-line view:

```bash
archon session abc123 --format human
```

The JSON output matches `types.Session`'s serialization directly, sharing the same CLI contract as `ps --json`. Automation consumers can compose with `jq` for field projection.

### Killing Sessions

`archon kill <id>` terminates a running session. On success it prints `ok`:

```bash
archon kill <session-id>
```

- Requires exactly one positional session id
- Prints `ok\n` on success, exits 0
- Errors produce a single-line stderr message and non-zero exit

### Interrupting Sessions

`archon interrupt <id>` stops the in-flight turn for a session without killing it. The session remains alive and ready for the next message:

```bash
archon interrupt <session-id>
```

- Requires exactly one positional session id
- Exits 0 with no output on success (silent success)
- If the session has no in-flight turn, the daemon treats it as a no-op and the CLI still exits 0
- Errors produce a single-line stderr message and non-zero exit
- Combine with `archon tail --follow <id>` to observe the interrupt taking effect

### Session Approvals

`archon approvals <id>` lists pending approvals for a session. `archon approve <id>` responds to a specific pending approval:

```bash
# List pending approvals (human table)
archon approvals <session-id>

# List pending approvals (machine-readable)
archon approvals <session-id> --json

# Approve a specific request
archon approve <session-id> --request-id 1 --decision allow_once

# Approve with responses
archon approve <session-id> --request-id 1 --decision allow --response "yes" --response "confirmed"

# Approve with accept-settings
archon approve <session-id> --request-id 1 --decision allow --accept-settings '{"remember":true}'
```

- `approvals`: exits 0 with a table (default) or JSON array (`--json`); empty list prints header only (or `[]` in JSON)
- `approve`: requires `--request-id` and `--decision`; optional `--response` (repeatable) and `--accept-settings` (JSON object)
- Decision strings are provider-specific (e.g. `allow_once`, `allow_always`, `deny`) — the CLI passes them through
- Compose with `jq`: `archon approvals <id> --json | jq '.[0].request_id'` to extract request ids

### Tail Snapshot

`archon tail <id>` prints a snapshot of recent output as a JSON array. Adding `--follow` (or `-f`) keeps the stream open and emits new events in real time as NDJSON:

```bash
# Snapshot (existing behaviour, unchanged)
archon tail <id> --lines 50

# Live stream — stays open until Ctrl-C or the session ends
archon tail <id> --follow

# Follow with backfill (emit last 20 lines, then stream live, deduplicating the overlap)
archon tail <id> -f --lines 20

# Follow a specific stream
archon tail <id> --follow --stream stderr
```

- `--follow` / `-f`: keep the stream open after the initial snapshot; new events are written as NDJSON lines, flushed immediately.
- `--stream`: selects which output stream to follow (default: `combined`; providers may expose `stdout`, `stderr`, etc.).
- When `--lines N` is set alongside `--follow`, the command emits the last N snapshot items first, then transitions to live events. If the first live event duplicates the last snapshot item, it is silently dropped.
- SIGINT (Ctrl-C) and SIGTERM produce a clean exit (code 0). If the daemon closes the stream, the command also exits cleanly.

### Send Messages

`archon send <id>` delivers a new user message into an existing session. By default it prints the returned `turn_id` on success so scripts can chain follow-up automation.

```bash
# Send plain text as a positional argument
archon send <id> "Please continue from the last step"

# Equivalent explicit flag form
archon send <id> --text "Summarize what you just did"

# Send structured input items from a file
archon send <id> --input-items items.json

# Send structured input items from stdin
cat items.json | archon send <id> --input-items -

# Emit the full daemon response as JSON
archon send <id> "hello" --json
```

Rules:
- Provide exactly one input form: positional text, `--text`, or `--input-items`
- `--input-items` accepts either a file path or `-` for stdin and must contain a JSON array
- `--json` prints the full `SendSessionResponse`; otherwise only `turn_id` is printed (if present)
- Flags may appear before or after the session id

### Cloud Login

Archon supports linking a local daemon to the Archon web app with a device-style login flow:

```bash
archon login
archon login --no-browser
archon whoami
archon logout
```

- `archon login` opens the browser when possible and always prints a fallback verification URL and user code as `Visit: <url>` and `Code: <code>`. On success it prints `Logged in as <email>` when the email is available. Pass `--no-browser` to skip browser open.
- `archon whoami` prints the current cloud link status. When unlinked it prints `not logged in`. When linked it prints user display name, email, and installation name on separate lines.
- `archon logout` unlinks cloud credentials and prints the daemon's result message (full or partial success).
- Cloud auth is stored separately from the local daemon bearer token.
- `~/.archon/token` remains local CLI <-> local daemon auth only.
- Cloud link state is stored in `~/.archon/cloud_auth.json`.

### Title Generation

When `[title_generation].provider` is set to `openrouter` and the API key resolves,
Archon generates titles asynchronously after session/workflow creation.

- Initial title behavior is unchanged (fallback title is shown immediately).
- AI title updates are compare-and-set: if the title changed in the meantime, the update is skipped.
- User-initiated renames remain locked.
- AI-generated title updates are treated as system updates and do not lock titles.
- Provider error logs are sanitized and do not include raw provider response bodies.

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
- optionally set `[guided_workflows.defaults]` to control auto-created workflow session provider/model/access/reasoning
- optionally set `[guided_workflows.defaults].resolution_boundary` to tune default checkpoint strictness for new runs
- tune `[guided_workflows.policy]` and `[guided_workflows.rollout]` guardrails as needed

Workflow templates are configurable via JSON:

- user templates live at `~/.archon/workflow_templates.json`
- if present, user templates fully replace built-in defaults (no merge)
- built-in defaults are used only when no user template file exists

Per-step runtime overrides (`runtime_options`) are optional on each workflow step:

- supported fields: `model`, `reasoning`, `access`
- when present, step overrides take priority over the session's current runtime options
- when omitted, the step inherits whatever runtime options are currently active on the session

Optional phase-end judges can be configured on any phase:

- add a `judge` object with a custom `prompt`
- the judge runs after that phase completes and before the next phase starts
- the judge must answer with JSON and Archon pauses the workflow with a decision-needed notification when the judge rejects the phase or returns invalid output

Example `workflow_templates.json` (custom replacement file):

```json
{
  "version": 1,
  "templates": [
    {
      "id": "repo_hardening_delivery",
      "name": "Repo Hardening Delivery",
      "description": "Plan and implement security/reliability hardening in phases.",
      "default_access_level": "on_request",
      "phases": [
        {
          "id": "phase_1_discovery",
          "name": "Discovery",
          "judge": {
            "prompt": "Review the discovery phase. Pass only if the plan is specific, prioritized, and grounded in evidence from the repository. Fail if important unknowns remain unresolved."
          },
          "steps": [
            {
              "id": "step_1_inventory",
              "name": "Inventory",
              "prompt": "Audit current risk areas and produce a prioritized hardening plan.",
              "runtime_options": {
                "model": "gpt-5.3-codex",
                "reasoning": "extra_high"
              }
            }
          ]
        },
        {
          "id": "phase_2_execution",
          "name": "Execution",
          "steps": [
            {
              "id": "step_2_harden",
              "name": "Implement hardening",
              "prompt": "Implement the approved hardening items with tests and clear commit messages.",
              "runtime_options": {
                "model": "gpt-5.4-codex",
                "reasoning": "high"
              }
            },
            {
              "id": "step_3_commit",
              "name": "Commit",
              "prompt": "Create conventional commits with concise rationale.",
              "runtime_options": {
                "model": "gpt-5.3-codex-spark",
                "reasoning": "low"
              }
            }
          ]
        }
      ]
    }
  ]
}
```

If you still want to use `solid_phase_delivery` when providing a custom file, include a template with that same `id` in your file.

Reusable definitions are optional and additive. You can define shared prompts, steps, and phase templates once, then reference them:

```json
{
  "version": 1,
  "definitions": {
    "prompts": {
      "quality_checks": "Run relevant tests and quality checks, fix what is reasonable, then rerun."
    },
    "steps": {
      "quality_checks": {
        "id": "quality_checks",
        "name": "quality checks",
        "prompt_ref": "quality_checks"
      }
    },
    "phase_templates": {
      "delivery": {
        "id": "phase_delivery",
        "name": "Delivery",
        "step_refs": ["quality_checks"]
      }
    }
  },
  "templates": [
    {
      "id": "feature_delivery",
      "name": "Feature Delivery",
      "phases": [
        {
          "phase_template_ref": "delivery"
        }
      ]
    }
  ]
}
```

When enabled, daemon exposes guided workflow lifecycle endpoints:

- `GET /v1/workflow-templates`
- `POST /v1/workflow-runs`
- `POST /v1/workflow-runs/:id/start`
- `POST /v1/workflow-runs/:id/pause`
- `POST /v1/workflow-runs/:id/resume`
- `POST /v1/workflow-runs/:id/decision`
- `GET /v1/workflow-runs/:id`
- `GET /v1/workflow-runs/:id/timeline`
- `POST /v1/workflow-runs/:id/stop`
- `POST /v1/workflow-runs/:id/rename`
- `POST /v1/workflow-runs/:id/dismiss`
- `POST /v1/workflow-runs/:id/undismiss`
- `GET /v1/workflow-runs/metrics`
- `POST /v1/workflow-runs/metrics/reset`

Workflow metrics and run history survive daemon restarts.

Manual start flow:

- from workspace/worktree/session context in the TUI, choose `Start Guided Workflow`
- choose a workflow template in the launcher
- configure run setup (workflow prompt + policy sensitivity)
- launch run and monitor the timeline/decision inbox surfaces

Checkpoint behavior:

- policy emits explicit decisions (`continue` or `pause`) with reasons and severity metadata
- pauses produce actionable decision-needed notifications (`approve_continue`, `request_revision`, `pause_run`)

`POST /v1/workflow-runs/:id/decision` accepts:

- `action`: `approve_continue` | `request_revision` | `pause_run`
- `decision_id` (optional)
- `note` (optional)

Troubleshooting:

- `guided workflows are disabled`: verify `[guided_workflows].enabled = true` and restart daemon
- `enter a workflow prompt before starting`: provide a feature/bug prompt in Run Setup before pressing `enter`
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
- `--scope core|ui|keybindings|workflow_templates|all`: print one scope or multiple scopes (repeatable)

Examples:

```bash
archon config --scope core
archon config --scope ui --format toml
archon config --scope keybindings --default
archon config --scope workflow_templates
archon config --scope core --scope keybindings --format json
```

`archon config --scope workflow_templates --default` returns the built-in default workflow templates.

### Daemon Control

`archon daemon` manages the local daemon process:

```bash
# Start in foreground (blocks)
archon daemon

# Start in background (logs to ~/.archon/daemon.log)
archon daemon --background

# Stop the running daemon
archon daemon --kill

# Force restart (stop then start in foreground)
archon daemon --force

# Force restart in background
archon daemon --force --background
```

- `--kill` is safe when no daemon is running (no-op, exits 0)
- `--kill` exits without starting a new daemon
- `--force` stops any running daemon first, then starts a new one
- Errors produce a single-line stderr message and non-zero exit

### UI Launch

`archon ui` verifies daemon readiness and launches the terminal UI:

```bash
# Default: verify daemon version compatibility then launch
archon ui

# Restart daemon during version check
archon ui --restart-daemon

# Skip version check (still requires reachable daemon)
archon ui --ignore-daemon-mismatch
```

- Logging is configured to `~/.archon/ui.log` before launch
- Default path checks daemon version against CLI version
- `--ignore-daemon-mismatch` bypasses version enforcement but still requires a reachable daemon
- Errors produce a single-line stderr message and non-zero exit

Clipboard copy always tries the system clipboard first, then OSC52 as fallback.

## Keybinding Command IDs
The following command IDs are supported in `keybindings.json`:

- `ui.menu`
- `ui.openSettings`
- `ui.quit`
- `ui.toggleSidebar`
- `ui.toggleNotesPanel`
- `ui.toggleContextPanel`
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
- `ui.startGuidedWorkflow`
- `ui.openNotes`
- `ui.openChat`
- `ui.refresh`
- `ui.killSession`
- `ui.interruptSession`
- `ui.dismissSelection`
- `ui.undismissSession`
- `ui.toggleDismissed`
- `ui.toggleNotesWorkspace`
- `ui.toggleNotesWorktree`
- `ui.toggleNotesSession`
- `ui.toggleNotesAll`
- `ui.pauseFollow`
- `ui.toggleReasoning`
- `ui.toggleMessageSelect`
- `ui.inputClear`
- `ui.composeModel`
- `ui.composeReasoning`
- `ui.composeAccess`
- `ui.inputSubmit`
- `ui.inputNewline`
- `ui.inputLineUp`
- `ui.inputLineDown`
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
- `ui.rename`
- `ui.historyBack`
- `ui.historyForward`

`ui.dismissSession` is still accepted as a legacy alias for `ui.dismissSelection`.
`ui.composeClearInput` is still accepted as a legacy alias for `ui.inputClear`.
