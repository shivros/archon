# SOLID Plan: Typeahead Input Must Always Beat Hotkeys

## Problem Statement
When a typeahead-enabled picker is focused/open, plain user text input (including `h`, `j`, `k`, `l` and any alphanumeric) can still collide with app-level hotkey handling. This is most visible in the compose model selector, but the same risk exists in other picker flows.

Target behavior:
- In an active typeahead context, printable text must be treated as query input first.
- Global/context hotkeys must not preempt text entry in that context.
- Navigation and explicit control keys (`up/down`, `enter`, `esc`, `backspace`, clear command) must continue to work.

## Scope
In scope:
- Compose option picker (`model_reducers.go`)
- Guided workflow pickers and compose option picker usage (`model_guided_workflow.go`)
- Existing-worktree filter picker (`add_worktree_controller.go`)
- Group picker step handling (`group_picker_step.go`)
- Shared typeahead mechanics (`picker_typeahead.go`)
- Keyboard routing tests across above surfaces

Out of scope:
- Rebinding keymaps UX
- New feature flags or staged rollout controls

## Root Cause (Current Architecture)
Typeahead handling is present, but keyboard precedence is duplicated in multiple reducers/controllers. Some paths manually swallow text after attempting typeahead; others rely on ad hoc ordering between hotkeys and text. This violates SRP and makes behavior drift likely.

## SOLID Design Strategy

### 1) Single Responsibility Principle (SRP)
Introduce a single keyboard arbitration unit for picker contexts that decides key intent and precedence:
- Typeahead text
- Picker navigation
- Picker control commands
- Pass-through/non-picker commands

Reducers/controllers should delegate to this unit rather than re-encoding precedence rules.

### 2) Open/Closed Principle (OCP)
Design the arbiter with policy interfaces so new picker types can plug in behavior without modifying core routing:
- `PickerKeyPolicy` for control/navigation mapping
- `TypeaheadTextPolicy` for printable/modifier eligibility

### 3) Liskov Substitution Principle (LSP)
Any `queryPicker` implementation must behave correctly with the arbiter contract (append/backspace/clear/query semantics). No picker-specific assumptions in the core router.

### 4) Interface Segregation Principle (ISP)
Keep interfaces narrow:
- `queryPicker` remains focused on query operations
- `KeyResolver` remains focused on key normalization/command matches
- New policy interfaces should expose only what arbitration needs

### 5) Dependency Inversion Principle (DIP)
High-level reducers depend on abstraction (`PickerKeyboardArbiter`) rather than concrete key parsing details. Inject resolver/policies into arbiter construction.

## Implementation Plan (End-to-End)

### Phase 1: Establish Shared Keyboard Arbitration
1. Add `picker_keyboard_arbiter.go` in `internal/app`.
2. Define result type, e.g. `PickerKeyDecision`:
   - `Handled bool`
   - `RenderRequired bool` (for views needing rerender)
   - `Action` (optional enum: `none`, `move`, `confirm`, `cancel`, `queryChanged`)
3. Implement `Handle(msg tea.Msg, picker queryPicker, ctx PickerContext)` that enforces precedence:
   - First: explicit picker controls (`esc`, `enter`, navigation keys)
   - Second: typeahead editing (`append`, `backspace`, `clear`)
   - Third: consume printable text in picker context even if query unchanged
   - Else: unhandled
4. Consolidate printable-text eligibility in one function (single source of truth):
   - Printable/no disallowed modifiers => text input candidate
   - Command combos remain commands (ctrl/super/alt handling per policy)

### Phase 2: Wire Call Sites to Arbiter
1. Refactor compose picker handling in `reduceComposeInputKey` to use arbiter decisions.
2. Refactor guided workflow picker branches in `reduceGuidedWorkflowMode` to use same arbiter.
3. Refactor existing-worktree filtering in `AddWorktreeController.handleKey` to use same arbiter.
4. Refactor `groupPickerStepHandler.Update` to use same arbiter path.
5. Remove duplicated manual text-swallow and key-order code where replaced.

### Phase 3: Preserve Existing Non-Text Semantics
1. Keep current behavior for:
   - `j/k` and arrow navigation
   - `esc` clear-then-close flows
   - `enter` selection/confirm behavior
   - clear command keybinding overrides (`KeyCommandInputClear`)
2. Ensure global hotkeys still work when picker is not active.

### Phase 4: Test Matrix (Required Before Merge)
Add/extend tests in:
- `internal/app/picker_typeahead_test.go`
- `internal/app/compose_runtime_options_test.go`
- `internal/app/model_guided_workflow_test.go`
- `internal/app/add_input_controller_test.go`
- `internal/app/edit_workspace_controller_test.go` and/or group picker tests where relevant

Required scenarios:
1. Typing `h`, `j`, `k`, `l` updates query in each picker context.
2. Typing alphanumeric keys that are global hotkeys does not trigger hotkey behavior while picker active.
3. Ctrl/super command keys still trigger mapped commands (e.g., clear override) where intended.
4. Navigation keys still move selection and do not pollute query.
5. `esc` clear-then-close semantics preserved.
6. `enter` selects current filtered option.
7. Pasted text still normalizes and filters.
8. Behavior when query result set is unchanged (input still consumed; no hotkey side effect).

### Phase 5: Regression Validation
1. Run focused app tests:
   - `go test ./internal/app/... -run "TypeAhead|Picker|ComposeOption|GuidedWorkflow|AddWorktree|GroupPicker"`
2. Run full app package tests:
   - `go test ./internal/app/...`
3. Manual smoke check:
   - Compose model selector (`ctrl+1`) type `h/j/k/l` and hotkey letters.
   - Guided workflow launcher/provider/policy filter typing.
   - Add worktree existing mode filter typing.
   - Group picker filter in add/edit workspace flows.

## No-Flag Release Strategy
Direct ship in one merge to main (no feature flags, no progressive rollout). Safety comes from:
- Shared arbitration abstraction reducing divergence
- Multi-surface regression tests
- Manual keyboard smoke checks before merge

## Acceptance Criteria
- In every active picker/typeahead context, printable text always updates/targets typeahead and never executes conflicting hotkeys.
- Existing picker control/navigation UX remains unchanged.
- All targeted tests pass, including new coverage for `h/j/k/l` and alphanumeric hotkey conflicts.
- No feature flags introduced.

## Risks and Mitigations
1. Risk: Over-consuming keys may block legitimate shortcuts.
   - Mitigation: explicit policy for modifier-based command keys and tests for remapped clear/submit commands.
2. Risk: Behavioral drift across reducers if partial migration occurs.
   - Mitigation: complete migration of all identified picker surfaces in this change.
3. Risk: Hidden picker surfaces missed.
   - Mitigation: repository search for `applyPickerTypeAhead`, `pickerTypeAheadText`, and `newPickerTypeAheadController` as part of PR checklist.

## PR Checklist
- [ ] Arbiter introduced and wired to all picker contexts
- [ ] Duplicated precedence logic removed
- [ ] Tests added/updated for all affected surfaces
- [ ] `go test ./internal/app/...` green
- [ ] Manual keyboard smoke checks completed
