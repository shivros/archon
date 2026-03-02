# Fix: Typeahead Input Conflict with Hotkeys

## Problem Statement

When typing in the model selector typeahead (or any compose option picker), keys like `h`, `j`, `k`, `l` (and other alphanumeric characters mapped to hotkeys) are being consumed incorrectly. Users cannot type these characters into the typeahead filter.

### Root Cause

In `model_reducers.go:589-592`, when the compose option picker is open:

```go
if pickerTypeAheadText(msg) != "" {
    // Consume plain text input while picker is open, even if query does not change.
    return true, nil
}
```

This unconditionally consumes ALL text input while the picker is open, regardless of whether the query actually changed. The intent was to prevent hotkey conflicts, but this breaks typing in the input field.

The flow:
1. User types "h" in model selector typeahead
2. `reduceComposeInputKey` handles the key first
3. Typeahead tries to append "h" to query - may or may not change query
4. Regardless of query change, the key is consumed (returned as "handled")
5. User cannot type "h"

### Affected Components

- Model selector typeahead (`composeOptionModel`)
- Reasoning selector typeahead (`composeOptionReasoning`)
- Access selector typeahead (`composeOptionAccess`)
- Potentially any picker using `composeOptionQueryPicker`

---

## SOLID-Compliant Implementation Plan

### Single Responsibility Principle (SRP)

**Issue**: `reduceComposeInputKey` in `model_reducers.go` handles too many concerns:
- Chat input key handling
- Compose option picker navigation
- Typeahead input consumption
- Hotkey handling for compose mode

**Solution**: Extract typeahead input handling into a dedicated function with clear responsibility.

### Open/Closed Principle (OCP)

**Issue**: The current implementation modifies behavior through conditional checks rather than extension.

**Solution**: Create a `TypeaheadInputHandler` interface that can be extended for different picker behaviors without modifying core reducer logic.

### Liskov Substitution Principle (LSP)

**Issue**: The `queryPicker` interface is used generically but the typeahead behavior is hardcoded.

**Solution**: Ensure any implementation of `queryPicker` can substitute without breaking typeahead behavior.

### Interface Segregation Principle (ISP)

**Issue**: `queryPicker` interface mixes query manipulation with what should be separate concerns.

**Solution**: Consider splitting into `QueryModifier` and `QueryObserver` interfaces.

### Dependency Inversion Principle (DIP)

**Issue**: High-level `reduceComposeInputKey` depends on low-level `pickerTypeAheadController` details.

**Solution**: Depend on abstractions (interfaces) rather than concrete implementations.

---

## Implementation Steps

### Step 1: Modify Typeahead Consumption Logic

**File**: `internal/app/model_reducers.go`

**Change**: Update lines 585-592 to only consume keys that actually modify the query OR are modifier keys.

Current code:
```go
composePicker := composeOptionQueryPicker{model: m}
if m.applyPickerTypeAhead(msg, composePicker) {
    return true, nil
}
if pickerTypeAheadText(msg) != "" {
    // Consume plain text input while picker is open, even if query does not change.
    return true, nil
}
```

New code:
```go
composePicker := composeOptionQueryPicker{model: m}
queryBefore := composePicker.Query()
if m.applyPickerTypeAhead(msg, composePicker) {
    queryAfter := composePicker.Query()
    if queryBefore != queryAfter {
        return true, nil
    }
}
if pickerTypeAheadText(msg) != "" {
    key := msg.Key()
    if key.Mod.Contains(tea.ModCtrl) || key.Mod.Contains(tea.ModSuper) || key.Mod.Contains(tea.ModAlt) {
        return true, nil
    }
}
```

**Rationale**:
- Only consume text input if it actually changes the query
- Still consume modifier key combinations (ctrl+a, alt+x, etc.) to prevent hotkey conflicts
- Allow plain alphanumeric characters to pass through if they don't modify the query

### Step 2: Add Navigation Key Check

**File**: `internal/app/model_reducers.go`

**Change**: Ensure navigation keys (j/k for up/down) are explicitly handled BEFORE typeahead processing.

Current lines 578-583 already handle j/k, but we should verify they return properly and don't fall through to typeahead:

```go
case "j", "down":
    m.moveComposeOptionPicker(1)
    return true, nil
case "k", "up":
    m.moveComposeOptionPicker(-1)
    return true, nil
```

These are correct - they return before reaching the typeahead logic.

### Step 3: Add Tests

**File**: `internal/app/model_reducers_test.go`

Add tests to verify:
1. Typing h/j/k/l in typeahead actually appends to query
2. Modifier keys (ctrl+h, alt+x) are still consumed
3. Query filtering works correctly (typing "cl" filters to "claude", etc.)
4. Backspace works correctly

### Step 4: Verify All Pickers

**Files to review**:
- `internal/app/chat_input_addon_controller.go` - Ensure all pickers use same logic
- `internal/app/select_picker.go` - Verify query append behavior
- Any other picker implementations

**Change**: If other pickers have similar unconditional consumption, apply same fix.

---

## Verification Plan

### Manual Testing

1. Open chat input (press `c` in sidebar)
2. Open model selector (press `ctrl+1`)
3. Type "claude" - should filter to Claude models
4. Type "gpt" - should filter to GPT models
5. Type "h", "j", "k", "l" individually - should appear in query
6. Type "ctrl+g" - should NOT appear in query (consumed as hotkey)
7. Type "ctrl+h" - should NOT appear in query (backspace with ctrl)
8. Navigate with j/k - should move selection, not affect query
9. Press enter to select - should work
10. Press esc to close - should work

### Automated Tests

Run existing tests:
```bash
go test ./internal/app/... -v -run "Typeahead|Picker"
```

Run new tests:
```bash
go test ./internal/app/... -v -run "TestTypeahead"
```

---

## Risk Assessment

- **Low Risk**: Changes are isolated to input consumption logic
- **No Breaking Changes**: Only affects behavior inside typeahead, not hotkey behavior in other contexts
- **Backward Compatible**: Same UI/UX, just fixes broken input

---

## Timeline

- Step 1 (modify logic): 30 minutes
- Step 2 (verify navigation): 15 minutes
- Step 3 (add tests): 45 minutes
- Step 4 (verify all pickers): 30 minutes
- Manual testing: 20 minutes

**Total Estimate**: 2.5 hours

---

## Files to Modify

1. `internal/app/model_reducers.go` - Core fix at lines 585-592
2. `internal/app/model_reducers_test.go` - Add tests
3. `internal/app/chat_input_addon_controller.go` - Verify picker usage (if needed)
