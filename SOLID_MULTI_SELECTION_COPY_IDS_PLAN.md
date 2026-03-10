# SOLID Plan: Multi-Selection Copy for Workspace + Workflow + Session IDs

## 1. Goal

Enable `copy id` to copy all relevant identifiers for the current sidebar selection set in one operation, one identifier per line, with deterministic ordering.

Target example output:

```
/home/shiv/Workspaces/shivros/archon/main
gwf-a9260fca71345cdd
019cd664-5230-7620-929b-51a1c710c1da
019cc94f-16f3-7d10-adc0-7f9120edab35
```

Direct cutover only. No feature flags and no progressive rollout.

## 2. Product Contract

1. If multi-select is non-empty, copy identifiers for selected items only.
2. If multi-select is empty, copy identifier for the focused sidebar item (current behavior fallback pattern via `SelectedItemsOrFocused`).
3. Identifier mapping:
   - Workspace -> `workspace.RepoPath` (path is the user-facing workspace identifier in this flow).
   - Workflow -> `workflow run ID`.
   - Session -> `session ID`.
4. Output format: newline-delimited, one identifier per line, no decoration.
5. Ordering: same order as visible sidebar order (top-to-bottom), not map iteration order.
6. Unsupported selected kinds (for example recents/worktree) are ignored.
7. If nothing copyable is selected/focused, show warning status and do not enqueue clipboard command.

## 3. Current State (Relevant Code)

- Keybinding and command identity:
  - `internal/app/keybindings.go` (`KeyCommandCopySessionID`, default `ctrl+g`).
  - `internal/app/hotkeys.go` (hotkey label currently "copy id").
- Keyboard reducers:
  - `internal/app/model_key_actions.go` -> `reduceClipboardAndSearchKeys`.
  - `internal/app/model_reducers.go` -> compose input pre-handle path also copies session ID on `ctrl+g`.
- Selection model:
  - `internal/app/sidebar_controller.go` has `SelectedItemsOrFocused`, `SelectedItems`, and ordered selection reads.
  - `internal/app/model.go` already exposes `selectedItemsOrFocused()`.
- Existing copy primitive:
  - `internal/app/clipboard.go` -> `copyWithStatusCmd(text, success)`.

Gap: copy logic is session-only and duplicated across normal-mode and compose-mode reducers.

## 4. SOLID Design

## 4.1 New Abstractions

Introduce a dedicated selection-copy module in `internal/app/selection_copy.go`:

1. `SelectionCopyValueResolver` interface:
   - `Resolve(*sidebarItem) (value string, ok bool)`
2. `SelectionCopyPayloadBuilder` interface:
   - `Build(items []*sidebarItem) (payload string, copiedCount int, skippedCount int)`

Default implementation:
- Resolver chain:
  - workspace resolver
  - workflow resolver
  - session resolver
- Payload builder:
  - deduplicates by sidebar item key
  - preserves incoming order
  - joins values with `"\n"`

## 4.2 Model Integration

Add one orchestration method on model (for both key paths):

- `copySidebarSelectionIDsCmd() tea.Cmd`
  - gets `items := m.selectedItemsOrFocused()`
  - uses injected/default payload builder
  - sets warning if `copiedCount == 0`
  - returns `m.copyWithStatusCmd(payload, fmt.Sprintf("copied %d id(s)", copiedCount))`

Use this method in:
1. `reduceClipboardAndSearchKeys` in `internal/app/model_key_actions.go`
2. compose `ctrl+g` path in `internal/app/model_reducers.go`

This removes duplication and keeps a single copy behavior contract.

## 4.3 Dependency Inversion

Follow existing model option pattern (same style as selection-operation planner/executor):

1. Add field on `Model`:
   - `selectionCopyPayloadBuilder SelectionCopyPayloadBuilder`
2. Add option:
   - `WithSelectionCopyPayloadBuilder(builder SelectionCopyPayloadBuilder) ModelOption`
3. Add default resolver:
   - `selectionCopyPayloadBuilderOrDefault()`

This keeps copy behavior testable and replaceable without modifying reducers.

## 5. Implementation Plan (End-to-End)

## Phase 1: Selection Copy Domain Module

Files:
- `internal/app/selection_copy.go` (new)

Work:
1. Implement resolver interfaces and default resolvers.
2. Implement payload builder with:
   - stable order
   - unique sidebar-item key filtering
   - newline output.
3. Define clear handling for empty values:
   - workspace with empty repo path is skipped.
4. Add model wiring helper methods in this file (or sibling file if preferred).

## Phase 2: Wire Keyboard Entry Points

Files:
- `internal/app/model_key_actions.go`
- `internal/app/model_reducers.go`

Work:
1. Replace inline session-only copy logic with `m.copySidebarSelectionIDsCmd()`.
2. Keep keybinding command identity unchanged (`KeyCommandCopySessionID`) to preserve existing user config compatibility.
3. Keep behavior synchronous from reducer perspective (returns handled with nil/clipboard cmd as today).

## Phase 3: UX Text Alignment

Files:
- `internal/app/hotkeys.go` (optional label-only update)

Work:
1. Update displayed hotkey label from "copy id" to "copy selected ids" (or keep concise variant if preferred).
2. Update warning/success strings for clarity:
   - warning: `no workspace/workflow/session selected`
   - success: `copied N id(s)`

## Phase 4: Tests

## 4.1 New Unit Tests (selection copy module)

File:
- `internal/app/selection_copy_test.go` (new)

Cases:
1. Mixed selection: workspace + workflow + sessions -> newline payload in sidebar order.
2. Multi-select present excludes focused non-selected item.
3. Empty multi-select falls back to focused item.
4. Unsupported items skipped.
5. Empty workspace repo path skipped.
6. Duplicate sidebar item keys are deduped.

## 4.2 Reducer Integration Tests

File:
- `internal/app/model_reducers_test.go`

Cases:
1. `ctrl+g` with mixed selected items builds non-nil clipboard cmd and no warning.
2. `ctrl+g` no copyable selection sets warning and returns nil cmd.
3. Remapped copy key still triggers unified selection-copy path.
4. Compose input mode `ctrl+g` uses the same unified selection-copy behavior.

## 4.3 Regression Tests (optional but recommended)

File:
- `internal/app/model_context_menu_actions_test.go`

Case:
1. Ensure existing context-menu single-item copy actions still behave unchanged.

## Phase 5: Verification

Run:
1. `go test ./internal/app -run SelectionCopy`
2. `go test ./internal/app -run CopySessionID`
3. `go test ./internal/app`

Manual sanity:
1. Select workspace + workflow + 2 sessions with `space`.
2. Press `ctrl+g`.
3. Paste clipboard and verify four lines in visual sidebar order.

## 6. SOLID Mapping

1. SRP
   - Reducers only handle key routing.
   - Copy payload construction lives in a dedicated module.
2. OCP
   - New sidebar kinds can be supported by adding resolvers without changing reducer logic.
3. LSP
   - Any custom payload builder/resolver can substitute defaults via model options.
4. ISP
   - Small interfaces (`Resolve`, `Build`) avoid forcing unrelated methods.
5. DIP
   - Model depends on `SelectionCopyPayloadBuilder` abstraction, not concrete resolver implementation.

## 7. Risks and Mitigations

1. Risk: Ambiguity around workspace identifier (path vs workspace ID).
   - Mitigation: codify workspace copy as `RepoPath` in tests and docs for this feature.
2. Risk: Behavior drift between normal mode and compose mode.
   - Mitigation: both paths call one model method (`copySidebarSelectionIDsCmd`).
3. Risk: Ordering nondeterminism.
   - Mitigation: consume `SelectedItemsOrFocused` order and avoid map iteration.

## 8. Acceptance Criteria

1. Multi-selection copy produces newline-delimited values for workspace path + workflow IDs + session IDs.
2. Output order matches sidebar visual order.
3. Single focused workspace/workflow/session copy works with same hotkey.
4. Unsupported sidebar kinds do not crash and are skipped.
5. No feature flags; behavior is the default immediately after merge.
