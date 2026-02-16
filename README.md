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

Configuration is file-based:

- `config.toml` controls daemon/core behavior (daemon address, provider defaults, logging/debug settings).
- `ui.toml` controls UI-level settings.
- `keybindings.json` overrides UI hotkeys.

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
