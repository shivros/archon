# Plan: Full Non-Codex Backend Support for Guided Workflows

## Executive Summary

Guided workflows currently only work with `codex` and `opencode` providers. This plan outlines the changes needed to support Claude, Gemini, and custom backends.

---

## Current State Analysis

### How Guided Workflows Work

1. **Step Prompt Dispatch** (`internal/daemon/guided_workflows_bridge.go:595-602`)
   - The workflow engine sends prompts to sessions via `StepPromptDispatcher`
   - Only `codex` and `opencode` are allowed (hard-coded allowlist)

2. **Turn Completion Signaling**
   - Workflows wait for `turn/completed` events to advance to the next step
   - Each turn has a unique `turnID` for tracking

3. **Provider Capabilities** (`internal/providers/registry.go`)

| Provider | UsesItems | SupportsEvents | SupportsInterrupt | NoProcess |
|----------|-----------|----------------|-------------------|-----------|
| codex    | false     | true           | true              | false     |
| claude   | true      | false          | false             | true      |
| opencode | true      | true           | true              | true      |
| kilocode | true      | true           | true              | true      |
| gemini   | false     | false          | false             | false     |
| custom   | false     | false          | false             | false     |

### Why Codex/OpenCode Work

- **Codex**: Uses JSON-RPC with `turn/start` and `turn/completed` events. Each turn has a unique ID returned from `turn/start`.
- **OpenCode**: Has an HTTP event stream that emits `turn/completed` events with turn tracking.

### Why Claude/Gemini Don't Work

1. **No `turn/completed` Events**: Claude CLI runs synchronously per message. It doesn't emit structured events.
2. **Empty Turn IDs**: `claudeConversationAdapter.SendMessage` returns `""` for turnID (line 334).
3. **Immediate Turn Completion**: Publishes `turn/completed` immediately after send (line 333) - before the response is complete.
4. **No Event Subscription**: `claudeConversationAdapter.SubscribeEvents` returns an error (line 337-339).
5. **Provider Allowlist**: `guidedWorkflowProviderSupportsPromptDispatch` only allows `codex` and `opencode`.

---

## Implementation Plan

### Phase 1: Foundation - Turn ID Generation for Synchronous Providers

**Goal**: Enable Claude and similar synchronous providers to generate and track turn IDs.

#### 1.1 Add Turn ID Generation

**File**: `internal/daemon/session_conversation_adapters.go`

```go
// For claudeConversationAdapter.SendMessage:
// - Generate a UUID turnID before sending
// - Return the turnID
// - Publish turn/completed AFTER the CLI completes (not immediately)
```

**Changes**:
- Generate `turnID` using UUID or nanoid
- Track active turns in a map for the session
- Return the turnID from `SendMessage`
- Move `publishTurnCompleted` call to after the CLI finishes

#### 1.2 Update Session Meta Storage

**File**: `internal/daemon/session_conversation_adapters.go`

Store the turn ID in `SessionMeta`:
```go
_, _ = service.stores.SessionMeta.Upsert(ctx, &types.SessionMeta{
    SessionID:    session.ID,
    LastTurnID:   turnID,  // Store the generated turn ID
    LastActiveAt: &now,
})
```

---

### Phase 2: Asynchronous Turn Completion Detection

**Goal**: Detect when Claude has finished processing and emit the turn/completed event.

#### 2.1 Option A: Process-Based Detection (Recommended for Claude)

The Claude CLI runs synchronously. When `cmd.Wait()` returns, the turn is complete.

**File**: `internal/daemon/provider_claude.go`

Modify `claudeRunner.run()` to:
1. Generate a turnID before starting
2. Pass turnID to a callback when complete
3. Emit `turn/completed` event via `ProviderItemSink`

```go
type claudeRunner struct {
    // ... existing fields ...
    onTurnCompleted func(turnID string)  // New callback
}

func (r *claudeRunner) run(text string, runtimeOptions *types.SessionRuntimeOptions) error {
    turnID := generateTurnID()

    // ... existing command execution ...

    err = cmd.Wait()
    wg.Wait()

    // Emit turn completion
    if r.onTurnCompleted != nil {
        r.onTurnCompleted(turnID)
    }

    return err
}
```

#### 2.2 Option B: Polling-Based Detection

For providers without event streams, implement polling:
- Poll the provider's conversation endpoint
- Detect when the response is complete
- Emit `turn/completed`

#### 2.3 Option C: Item-Based Detection

Claude uses `UsesItems: true`. We can detect turn completion by:
- Watching for `assistant` message items
- When a complete assistant message appears, emit `turn/completed`

---

### Phase 3: Provider Allowlist Extension

**Goal**: Remove hard-coded allowlist and use capability-based detection.

#### 3.1 Update `guidedWorkflowProviderSupportsPromptDispatch`

**File**: `internal/daemon/guided_workflows_bridge.go`

Replace:
```go
func guidedWorkflowProviderSupportsPromptDispatch(provider string) bool {
    switch strings.ToLower(strings.TrimSpace(provider)) {
    case "codex", "opencode":
        return true
    default:
        return false
    }
}
```

With capability-based check:
```go
func guidedWorkflowProviderSupportsPromptDispatch(provider string) bool {
    caps := providers.CapabilitiesFor(provider)
    // Provider must support turn completion signaling
    // Either via events (codex/opencode) or via items (claude)
    return caps.SupportsEvents || caps.UsesItems
}
```

Or add a new capability:
```go
type Capabilities struct {
    // ... existing fields ...
    SupportsTurnTracking bool  // New field
}
```

#### 3.2 Add Capability Declaration

**File**: `internal/providers/registry.go`

```go
// For Claude
{
    Name:    "claude",
    Label:   "claude",
    Runtime: RuntimeClaude,
    CommandCandidates: []string{"claude"},
    Capabilities: Capabilities{
        UsesItems:            true,
        NoProcess:            true,
        SupportsTurnTracking: true,  // New
    },
}
```

---

### Phase 4: Event Emission for Non-Event Providers

**Goal**: Make Claude emit `turn/completed` events compatible with the notification system.

#### 4.1 Add Turn Event Emission to Claude Provider

**File**: `internal/daemon/provider_claude.go`

Add event emission via `ProviderItemSink`:

```go
func (r *claudeRunner) emitTurnCompleted(turnID string) {
    if r.items == nil {
        return
    }
    r.items.Append(map[string]any{
        "type": "event",
        "method": "turn/completed",
        "params": map[string]any{
            "turn": map[string]any{
                "id":     turnID,
                "status": "completed",
            },
        },
    })
}
```

#### 4.2 Update Session Event Handling

**File**: `internal/daemon/session_conversation_adapters.go`

Ensure the session adapter recognizes turn events from items:
- Parse items for `turn/completed` events
- Call `publishTurnCompleted` when detected

---

### Phase 5: Gemini Support

**Goal**: Enable Gemini provider for guided workflows.

#### 5.1 Evaluate Gemini CLI Capabilities

Research needed:
- Does Gemini CLI support streaming output?
- Does it have session/conversation IDs?
- Can we detect turn completion?

#### 5.2 Implement Gemini Provider Enhancement

Based on findings, implement similar to Phase 1-4 for Claude.

---

### Phase 6: Custom Provider Support

**Goal**: Allow configuration-based provider support.

#### 6.1 Configuration Schema

**File**: `internal/config/settings.go`

```toml
[[providers.custom]]
name = "my-provider"
command = "my-llm-cli"
capabilities = ["turn_tracking", "items"]
```

#### 6.2 Dynamic Provider Registration

Allow runtime registration of custom providers with capability declarations.

---

## Implementation Order

| Phase | Priority | Effort | Dependencies |
|-------|----------|--------|--------------|
| 1.1-1.2 Turn ID Generation | High | 2h | None |
| 2.1 Process-Based Detection | High | 4h | Phase 1 |
| 3.1-3.2 Allowlist Extension | High | 2h | Phase 2 |
| 4.1-4.2 Event Emission | Medium | 3h | Phase 2 |
| 5.1-5.2 Gemini Support | Low | 6h | Phase 1-4 |
| 6.1-6.2 Custom Providers | Low | 8h | Phase 1-4 |

**Total Estimated Effort**: 25 hours

---

## Testing Strategy

### Unit Tests

1. Test turn ID generation and uniqueness
2. Test turn completion event emission
3. Test capability-based provider selection
4. Test `publishTurnCompleted` with generated turn IDs

### Integration Tests

1. Create workflow run with Claude provider
2. Verify step prompts are dispatched
3. Verify turn completion advances workflow
4. Verify checkpoint decisions work

### Manual Testing

1. Configure Claude as default provider
2. Run guided workflow with multiple steps
3. Verify each step completes correctly
4. Test interrupt and resume scenarios

---

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Claude CLI doesn't support interruption | High | Medium | Document limitation, degrade gracefully |
| Turn completion timing is unreliable | Medium | High | Add timeout-based fallback |
| Gemini CLI has different architecture | Medium | Medium | Phase 5 research first |
| Breaking existing codex/opencode flows | Low | High | Comprehensive test coverage |

---

## Open Questions

1. **Claude Interruption**: Claude CLI runs synchronously. Should we:
   - Kill the subprocess on interrupt?
   - Mark as unsupported and document limitation?

2. **Turn ID Format**: Should we use:
   - UUID v4?
   - nanoid?
   - Provider-native IDs where available?

3. **Backward Compatibility**: Do we need to support existing Claude sessions that don't have turn tracking?

4. **Gemini Research**: What are Gemini CLI's actual capabilities? Requires investigation.

---

## Success Criteria

1. ✅ Guided workflows start with Claude provider without error
2. ✅ Each workflow step dispatches prompts to Claude sessions
3. ✅ Turn completion is detected and advances workflow
4. ✅ Checkpoint decisions work correctly
5. ✅ Workflow runs complete successfully
6. ✅ Codex and OpenCode providers continue to work unchanged
7. ✅ Configuration allows setting Claude as default provider for workflows
