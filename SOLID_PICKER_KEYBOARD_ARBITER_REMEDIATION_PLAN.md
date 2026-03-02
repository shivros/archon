# SOLID Remediation Plan: Picker Keyboard Arbitration

## Objective
Address the SOLID audit findings by:
1. Removing unused `queryChanged` signal or making it a meaningful contract.
2. Eliminating duplicated picker keyboard arbitration across reducers/controllers.
3. Decoupling picker flows from raw `msg.String()` checks via resolver-based abstractions.

## Findings to Resolve
- Dead internal result field (`queryChanged`) creates unnecessary surface area.
- Picker navigation/control/typeahead precedence is duplicated across multiple call sites.
- Some picker handlers still use raw key strings instead of injected key resolvers.

## Design Principles (SOLID)
- **SRP**: One component owns picker keyboard arbitration.
- **OCP**: New picker contexts plug into policies, not copy/pasted switch statements.
- **LSP**: Any `queryPicker` can be routed through the same arbiter contract.
- **ISP**: Keep interfaces narrow (`queryPicker`, `KeyResolver`, small policy interfaces).
- **DIP**: Reducers/controllers depend on arbiter interfaces, not key parsing details.

## Target Architecture
Introduce a shared arbiter module in `internal/app`:
- `PickerKeyboardArbiter` (core coordinator)
- `PickerNavigationPolicy` (up/down/page/home/end/toggle semantics)
- `PickerControlPolicy` (esc/enter/cancel/confirm behavior)
- Existing `pickerTypeAheadController` reused for text/edit/paste handling

Core contract:
- `Handle(msg tea.Msg, ctx PickerKeyboardContext) PickerKeyboardDecision`
- `PickerKeyboardDecision` contains only fields with active consumers:
  - `Handled bool`
  - `Cmd tea.Cmd` (optional)
  - `Render bool` (optional, only if consumed)

## Phase Plan

### Phase 1: Remove Dead Contract Surface
1. Remove `queryChanged` from `pickerTypeAheadResult` and simplify to either:
   - `bool` return only, or
   - decision struct with only used fields.
2. Update tests to validate behavior, not unused internals.

Deliverable:
- `picker_typeahead.go` has no dead result channels.

### Phase 2: Introduce Shared Arbiter
1. Add `picker_keyboard_arbiter.go` with:
   - Context object containing picker, key resolver functions, and callback hooks (`onCancel`, `onConfirm`, `onMove`, etc.).
   - Unified precedence order:
     1. Explicit control keys (`esc`, `enter`)
     2. Navigation keys (`up/down`, page, home/end as configured)
     3. Typeahead/paste/edit consumption
2. Keep typeahead consumption behavior unchanged: printable text is always consumed in active picker context.

Deliverable:
- Single reusable keyboard routing component for picker contexts.

### Phase 3: Migrate Call Sites to Arbiter (No Partial Ownership)
Migrate these paths to arbiter and remove duplicated switch blocks:
1. Compose option picker handling in `reduceComposeInputKey`.
2. Guided workflow picker handling in `reduceGuidedWorkflowMode`.
3. Workspace/group/provider/multi-select picker handling in `model_reducers.go`.
4. Group picker step handler.
5. Existing-worktree picker flow in add-worktree controller.
6. Note move picker flow.

Deliverable:
- Call sites delegate to arbiter + policies; no duplicated precedence logic remains.

### Phase 4: Resolver-Only Key Matching
1. Replace remaining raw `msg.String()` branches in picker routes with `keyString`/`keyMatchesCommand` injected via resolver.
2. Ensure command overrides (e.g., remapped clear) remain honored.

Deliverable:
- Picker keyboard handling is key-resolver driven everywhere.

### Phase 5: Tests and Verification
Add/adjust tests to validate shared behavior across all picker surfaces:
1. Printable text always consumed in active picker context.
2. `h/j/k/l` and alphanumeric hotkey letters update query instead of triggering hotkeys.
3. `up/down` navigation still works.
4. `esc` clear-then-cancel semantics preserved where applicable.
5. `enter` confirm/select semantics preserved.
6. Paste normalization behavior preserved.
7. Remapped command keys (clear/submit/etc.) still function.

Run:
- `go test ./internal/app/... -run "TypeAhead|Picker|Compose|GuidedWorkflow|AddWorktree|Group"`
- `go test ./internal/app/...`

## Acceptance Criteria
- No dead fields/signals in picker typeahead internals.
- One shared arbiter owns picker keyboard precedence.
- Picker flows do not directly depend on raw `msg.String()` for routing decisions.
- Existing functional behavior remains stable except intended precedence fixes.
- Full `internal/app` tests pass.

## Risks and Mitigations
1. **Risk**: Behavior drift during migration.
   - **Mitigation**: Migrate one picker surface at a time with parity tests before/after.
2. **Risk**: Over-generalization creates difficult abstractions.
   - **Mitigation**: Start with minimal policy interfaces; add only proven extension points.
3. **Risk**: Regressions in remapped keybindings.
   - **Mitigation**: Add explicit tests for resolver-driven remapped commands in each migrated surface.

## Execution Checklist
- [ ] Remove `queryChanged` dead contract.
- [ ] Implement `PickerKeyboardArbiter`.
- [ ] Migrate all listed picker surfaces.
- [ ] Replace raw-key routing with resolver-based routing.
- [ ] Expand regression tests.
- [ ] Pass focused + full app test suite.
