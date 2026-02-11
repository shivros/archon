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
    "opencode": { "prefix": "[OPN]", "color": "39" }
  }
}
```

`color` accepts Lip Gloss-compatible terminal colors (ANSI index like `"208"` or hex like `"#ff8a3d"`).
