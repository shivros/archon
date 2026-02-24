# SOLID-Compliant Plan: OpenCode Live Manager for Guided Workflows

## Problem Statement

Guided workflows for kilocode/opencode providers hang indefinitely because there's no persistent event subscription mechanism publishing `turn/completed` notifications. Codex has `CodexLiveManager` which maintains a live connection and publishes turn completion events. OpenCode/Kilocode lack this infrastructure.

---

## SOLID Analysis

### Single Responsibility Principle (SRP)

**Current Violation**: `CodexLiveManager` handles both:
1. Session lifecycle management (ensure, drop, persist)
2. Event broadcasting (hub, broadcast)
3. Turn coordination (start turn, probe activity)
4. Notification publishing (turn completed, approval required)

**Proposed Refactor**: Extract interfaces for each responsibility:

```
LiveSessionManager      - Session lifecycle (ensure, drop, cleanup)
EventBroadcaster        - Multi-subscriber event distribution
TurnCoordinator         - Turn start/wait/interrupt coordination
TurnCompletionPublisher - Notification publishing
```

### Open/Closed Principle (OCP)

**Current Violation**: Provider-specific logic is hard-coded:
- `CodexLiveManager.ensure()` only works with `codexAppServer`
- `startCodexAppServer()` is codex-specific

**Proposed Solution**: Provider-agnostic interfaces:
```go
type LiveSessionProvider interface {
    StartSession(ctx context.Context, session *types.Session, meta *types.SessionMeta) (LiveSession, error)
    ProviderName() string
}

type LiveSession interface {
    Events() <-chan types.CodexEvent
    StartTurn(ctx context.Context, input []map[string]any, opts *types.SessionRuntimeOptions) (string, error)
    Interrupt(ctx context.Context) error
    Respond(ctx context.Context, requestID int, result map[string]any) error
    Close()
}
```

### Liskov Substitution Principle (LSP)

**Requirement**: All provider implementations must be substitutable:
- `CodexLiveSession` implements `LiveSession`
- `OpenCodeLiveSession` implements `LiveSession`
- Both must handle all methods without throwing "not supported" for core operations

### Interface Segregation Principle (ISP)

**Current Issue**: `CodexLiveManager` exposes methods not all consumers need:
- `StartTurn` - only for workflow dispatch
- `Subscribe` - only for UI event streaming
- `Respond/Interrupt` - only for approval handling

**Proposed Interfaces**:

```go
// For workflow dispatch
type TurnStarter interface {
    StartTurn(ctx context.Context, session *types.Session, meta *types.SessionMeta, input []map[string]any, opts *types.SessionRuntimeOptions) (string, error)
}

// For UI event streaming
type EventStreamer interface {
    Subscribe(session *types.Session, meta *types.SessionMeta) (<-chan types.CodexEvent, func(), error)
}

// For approval handling
type ApprovalResponder interface {
    Respond(ctx context.Context, session *types.Session, meta *types.SessionMeta, requestID int, result map[string]any) error
}

// For turn interruption
type TurnInterrupter interface {
    Interrupt(ctx context.Context, session *types.Session, meta *types.SessionMeta) error
}
```

### Dependency Inversion Principle (DIP)

**Current Violation**: High-level modules depend on concrete implementations:
- `SessionService` depends on `*CodexLiveManager`
- `guidedWorkflowsPromptDispatcher` depends on `*CodexLiveManager`

**Proposed Solution**: Depend on abstractions:
```go
type LiveManager interface {
    TurnStarter
    EventStreamer
    ApprovalResponder
    TurnInterrupter
    SetNotificationPublisher(notifier NotificationPublisher)
}
```

---

## Architecture Design

### Phase 1: Extract Abstractions (No Behavior Change)

**Goal**: Define interfaces without changing existing behavior.

#### 1.1 Define Core Interfaces

**File**: `internal/daemon/live_session.go` (new file)

```go
package daemon

import (
    "context"
    "control/internal/types"
)

// LiveSession represents an active provider session with event streaming.
type LiveSession interface {
    // Events returns a channel for receiving session events.
    // The channel closes when the session terminates.
    Events() <-chan types.CodexEvent

    // Close terminates the session and releases resources.
    Close()

    // SessionID returns the managed session identifier.
    SessionID() string
}

// TurnCapableSession can start and manage turns.
type TurnCapableSession interface {
    LiveSession

    // StartTurn initiates a new turn with the given input.
    // Returns the turn ID for tracking.
    StartTurn(ctx context.Context, input []map[string]any, opts *types.SessionRuntimeOptions) (string, error)

    // Interrupt cancels the currently active turn.
    Interrupt(ctx context.Context) error

    // ActiveTurnID returns the currently executing turn, if any.
    ActiveTurnID() string
}

// ApprovalCapableSession can respond to approval requests.
type ApprovalCapableSession interface {
    LiveSession

    // Respond sends a decision for an approval request.
    Respond(ctx context.Context, requestID int, result map[string]any) error
}
```

#### 1.2 Define Provider Factory Interface

**File**: `internal/daemon/live_session_factory.go` (new file)

```go
package daemon

import (
    "context"
    "control/internal/types"
)

// LiveSessionFactory creates live sessions for providers.
type LiveSessionFactory interface {
    // Create establishes a live session for the given provider session.
    Create(ctx context.Context, session *types.Session, meta *types.SessionMeta) (LiveSession, error)

    // ProviderName returns the provider this factory supports.
    ProviderName() string
}

// TurnCapableSessionFactory creates turn-capable sessions.
type TurnCapableSessionFactory interface {
    LiveSessionFactory
    CreateTurnCapable(ctx context.Context, session *types.Session, meta *types.SessionMeta) (TurnCapableSession, error)
}
```

#### 1.3 Define Live Manager Interface

**File**: `internal/daemon/live_manager.go` (new file)

```go
package daemon

import (
    "context"
    "control/internal/types"
)

// LiveManager coordinates live sessions across providers.
type LiveManager interface {
    // StartTurn sends input to a session and returns the turn ID.
    StartTurn(ctx context.Context, session *types.Session, meta *types.SessionMeta, input []map[string]any, opts *types.SessionRuntimeOptions) (string, error)

    // Subscribe returns an event stream for the session.
    Subscribe(session *types.Session, meta *types.SessionMeta) (<-chan types.CodexEvent, func(), error)

    // Respond sends an approval decision.
    Respond(ctx context.Context, session *types.Session, meta *types.SessionMeta, requestID int, result map[string]any) error

    // Interrupt cancels an active turn.
    Interrupt(ctx context.Context, session *types.Session, meta *types.SessionMeta) error

    // SetNotificationPublisher configures the notification sink.
    SetNotificationPublisher(notifier NotificationPublisher)
}
```

### Phase 2: Refactor CodexLiveManager to Implement Interfaces

**Goal**: Make CodexLiveManager implement the new interfaces without breaking changes.

#### 2.1 CodexLiveSession Implements Interfaces

**File**: `internal/daemon/codex_live.go`

```go
// Ensure interface compliance at compile time.
var (
    _ LiveSession           = (*codexLiveSession)(nil)
    _ TurnCapableSession    = (*codexLiveSession)(nil)
    _ ApprovalCapableSession = (*codexLiveSession)(nil)
)

func (s *codexLiveSession) Events() <-chan types.CodexEvent {
    ch, _ := s.hub.Add()
    return ch
}

func (s *codexLiveSession) SessionID() string {
    return s.sessionID
}

func (s *codexLiveSession) StartTurn(ctx context.Context, input []map[string]any, opts *types.SessionRuntimeOptions) (string, error) {
    // Existing logic from codexAppServer.StartTurn
}

func (s *codexLiveSession) Interrupt(ctx context.Context) error {
    // Existing logic
}

func (s *codexLiveSession) ActiveTurnID() string {
    s.mu.Lock()
    defer s.mu.Unlock()
    return s.activeTurn
}

func (s *codexLiveSession) Respond(ctx context.Context, requestID int, result map[string]any) error {
    // Existing logic
}
```

#### 2.2 CodexLiveManager Uses Factory Pattern

**File**: `internal/daemon/codex_live_factory.go` (new file)

```go
package daemon

type codexLiveSessionFactory struct {
    stores *Stores
    logger logging.Logger
}

func newCodexLiveSessionFactory(stores *Stores, logger logging.Logger) *codexLiveSessionFactory {
    return &codexLiveSessionFactory{stores: stores, logger: logger}
}

func (f *codexLiveSessionFactory) ProviderName() string {
    return "codex"
}

func (f *codexLiveSessionFactory) CreateTurnCapable(ctx context.Context, session *types.Session, meta *types.SessionMeta) (TurnCapableSession, error) {
    // Extract session creation logic from CodexLiveManager.ensure()
}
```

### Phase 3: Implement OpenCodeLiveManager

**Goal**: Create opencode/kilocode equivalent of CodexLiveManager.

#### 3.1 OpenCode Live Session

**File**: `internal/daemon/opencode_live_session.go` (new file)

```go
package daemon

import (
    "context"
    "sync"
    "time"

    "control/internal/logging"
    "control/internal/types"
)

type openCodeLiveSession struct {
    mu           sync.Mutex
    sessionID    string
    providerName string
    providerID   string  // OpenCode session ID
    directory    string
    client       *openCodeClient
    events       <-chan types.CodexEvent
    cancelEvents func()
    hub          *codexSubscriberHub
    stores       *Stores
    notifier     NotificationPublisher
    closed       bool
}

var (
    _ LiveSession        = (*openCodeLiveSession)(nil)
    _ TurnCapableSession = (*openCodeLiveSession)(nil)
)

func (s *openCodeLiveSession) Events() <-chan types.CodexEvent {
    ch, _ := s.hub.Add()
    return ch
}

func (s *openCodeLiveSession) SessionID() string {
    return s.sessionID
}

func (s *openCodeLiveSession) StartTurn(ctx context.Context, input []map[string]any, opts *types.SessionRuntimeOptions) (string, error) {
    // OpenCode doesn't have turn IDs like codex
    // Generate one for tracking purposes
    turnID := generateTurnID()
    s.mu.Lock()
    s.activeTurn = turnID
    s.mu.Unlock()

    // Send via existing Prompt API
    text := extractTextInput(input)
    if err := s.client.Prompt(ctx, s.providerID, text, opts, s.directory); err != nil {
        s.mu.Lock()
        s.activeTurn = ""
        s.mu.Unlock()
        return "", err
    }

    return turnID, nil
}

func (s *openCodeLiveSession) Interrupt(ctx context.Context) error {
    return s.client.AbortSession(ctx, s.providerID, s.directory)
}

func (s *openCodeLiveSession) ActiveTurnID() string {
    s.mu.Lock()
    defer s.mu.Unlock()
    return s.activeTurn
}

func (s *openCodeLiveSession) Close() {
    s.mu.Lock()
    if s.closed {
        s.mu.Unlock()
        return
    }
    s.closed = true
    s.mu.Unlock()

    if s.cancelEvents != nil {
        s.cancelEvents()
    }
}

func (s *openCodeLiveSession) start() {
    go func() {
        defer s.Close()
        for event := range s.events {
            s.hub.Broadcast(event)

            if event.Method == "turn/completed" {
                s.mu.Lock()
                s.activeTurn = ""
                s.mu.Unlock()
                s.publishTurnCompleted(parseTurnIDFromEventParams(event.Params))
            }

            // Handle approval events
            if isApprovalMethod(event.Method) && event.ID != nil {
                s.storeApproval(event)
            }
        }
    }()
}

func (s *openCodeLiveSession) publishTurnCompleted(turnID string) {
    if s.notifier == nil {
        return
    }

    event := types.NotificationEvent{
        Trigger:    types.NotificationTriggerTurnCompleted,
        OccurredAt: time.Now().UTC().Format(time.RFC3339Nano),
        SessionID:  s.sessionID,
        TurnID:     turnID,
        Provider:   s.providerName,
        Source:     "opencode_live_event",
    }

    // Enrich with workspace/worktree from stores
    if s.stores != nil && s.stores.SessionMeta != nil {
        if meta, ok, _ := s.stores.SessionMeta.Get(context.Background(), s.sessionID); ok && meta != nil {
            event.WorkspaceID = meta.WorkspaceID
            event.WorktreeID = meta.WorktreeID
        }
    }

    s.notifier.Publish(event)
}
```

#### 3.2 OpenCode Live Factory

**File**: `internal/daemon/opencode_live_factory.go` (new file)

```go
package daemon

import (
    "context"
    "strings"

    "control/internal/config"
    "control/internal/types"
)

type openCodeLiveSessionFactory struct {
    providerName string
    stores       *Stores
    logger       logging.Logger
}

func newOpenCodeLiveSessionFactory(providerName string, stores *Stores, logger logging.Logger) *openCodeLiveSessionFactory {
    return &openCodeLiveSessionFactory{
        providerName: providerName,
        stores:       stores,
        logger:       logger,
    }
}

func (f *openCodeLiveSessionFactory) ProviderName() string {
    return f.providerName
}

func (f *openCodeLiveSessionFactory) CreateTurnCapable(ctx context.Context, session *types.Session, meta *types.SessionMeta) (TurnCapableSession, error) {
    providerID := ""
    if meta != nil {
        providerID = strings.TrimSpace(meta.ProviderSessionID)
    }
    if providerID == "" {
        return nil, invalidError("provider session id not available", nil)
    }

    client, err := newOpenCodeClient(resolveOpenCodeClientConfig(f.providerName, loadCoreConfigOrDefault()))
    if err != nil {
        return nil, err
    }

    events, cancel, err := client.SubscribeSessionEvents(ctx, providerID, session.Cwd)
    if err != nil {
        return nil, err
    }

    ls := &openCodeLiveSession{
        sessionID:    session.ID,
        providerName: f.providerName,
        providerID:   providerID,
        directory:    session.Cwd,
        client:       client,
        events:       events,
        cancelEvents: cancel,
        hub:          newCodexSubscriberHub(),
        stores:       f.stores,
    }
    ls.start()

    return ls, nil
}
```

### Phase 4: Unified Live Manager

**Goal**: Create a unified manager that delegates to provider-specific factories.

#### 4.1 CompositeLiveManager

**File**: `internal/daemon/composite_live_manager.go` (new file)

```go
package daemon

import (
    "context"
    "errors"
    "strings"
    "sync"

    "control/internal/logging"
    "control/internal/types"
)

// CompositeLiveManager routes live session operations to provider-specific implementations.
type CompositeLiveManager struct {
    mu         sync.Mutex
    factories  map[string]TurnCapableSessionFactory
    sessions   map[string]TurnCapableSession
    stores     *Stores
    logger     logging.Logger
    notifier   NotificationPublisher
}

func NewCompositeLiveManager(stores *Stores, logger logging.Logger, factories ...TurnCapableSessionFactory) *CompositeLiveManager {
    if logger == nil {
        logger = logging.Nop()
    }
    factoryMap := make(map[string]TurnCapableSessionFactory)
    for _, f := range factories {
        if f != nil {
            factoryMap[providers.Normalize(f.ProviderName())] = f
        }
    }
    return &CompositeLiveManager{
        factories: factoryMap,
        sessions:  make(map[string]TurnCapableSession),
        stores:    stores,
        logger:    logger,
    }
}

func (m *CompositeLiveManager) SetNotificationPublisher(notifier NotificationPublisher) {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.notifier = notifier
    // Propagate to existing sessions
    for _, s := range m.sessions {
        if ls, ok := s.(*openCodeLiveSession); ok {
            ls.notifier = notifier
        }
    }
}

func (m *CompositeLiveManager) StartTurn(ctx context.Context, session *types.Session, meta *types.SessionMeta, input []map[string]any, opts *types.SessionRuntimeOptions) (string, error) {
    if session == nil {
        return "", errors.New("session is required")
    }

    ls, err := m.ensure(ctx, session, meta)
    if err != nil {
        return "", err
    }

    return ls.StartTurn(ctx, input, opts)
}

func (m *CompositeLiveManager) Subscribe(session *types.Session, meta *types.SessionMeta) (<-chan types.CodexEvent, func(), error) {
    if session == nil {
        return nil, nil, errors.New("session is required")
    }

    ls, err := m.ensure(context.Background(), session, meta)
    if err != nil {
        return nil, nil, err
    }

    return ls.Events(), func() { ls.Close() }, nil
}

func (m *CompositeLiveManager) Respond(ctx context.Context, session *types.Session, meta *types.SessionMeta, requestID int, result map[string]any) error {
    // OpenCode doesn't use this path - approvals go through different mechanism
    return errors.New("provider does not support approval responses via live manager")
}

func (m *CompositeLiveManager) Interrupt(ctx context.Context, session *types.Session, meta *types.SessionMeta) error {
    if session == nil {
        return errors.New("session is required")
    }

    ls, err := m.ensure(ctx, session, meta)
    if err != nil {
        return err
    }

    return ls.Interrupt(ctx)
}

func (m *CompositeLiveManager) ensure(ctx context.Context, session *types.Session, meta *types.SessionMeta) (TurnCapableSession, error) {
    m.mu.Lock()
    defer m.mu.Unlock()

    // Check existing
    if ls := m.sessions[session.ID]; ls != nil {
        return ls, nil
    }

    // Find factory
    provider := providers.Normalize(session.Provider)
    factory := m.factories[provider]
    if factory == nil {
        return nil, errors.New("provider does not support live sessions")
    }

    // Create session
    ls, err := factory.CreateTurnCapable(ctx, session, meta)
    if err != nil {
        return nil, err
    }

    // Set notifier if available
    if ols, ok := ls.(*openCodeLiveSession); ok && m.notifier != nil {
        ols.notifier = m.notifier
    }

    m.sessions[session.ID] = ls
    return ls, nil
}

func (m *CompositeLiveManager) drop(sessionID string) {
    m.mu.Lock()
    defer m.mu.Unlock()
    if ls := m.sessions[sessionID]; ls != nil {
        ls.Close()
        delete(m.sessions, sessionID)
    }
}
```

### Phase 5: Wire into Existing Infrastructure

**Goal**: Replace direct `CodexLiveManager` usage with `CompositeLiveManager`.

#### 5.1 Update Daemon Initialization

**File**: `internal/daemon/daemon.go`

```go
// Before:
liveCodex := NewCodexLiveManager(d.stores, d.logger)

// After:
liveManager := NewCompositeLiveManager(
    d.stores,
    d.logger,
    newCodexLiveSessionFactory(d.stores, d.logger),
    newOpenCodeLiveSessionFactory("opencode", d.stores, d.logger),
    newOpenCodeLiveSessionFactory("kilocode", d.stores, d.logger),
)
```

#### 5.2 Update SessionService

**File**: `internal/daemon/session_service.go`

```go
type SessionService struct {
    manager    *SessionManager
    stores     *Stores
    live       LiveManager  // Changed from *CodexLiveManager
    logger     logging.Logger
    adapters   *conversationAdapterRegistry
    notifier   NotificationPublisher
    guided     guidedworkflows.RunService
}
```

#### 5.3 Update Conversation Adapters

**File**: `internal/daemon/session_conversation_adapters.go`

```go
func (a openCodeConversationAdapter) SendMessage(ctx context.Context, service *SessionService, session *types.Session, meta *types.SessionMeta, input []map[string]any) (string, error) {
    // Use live manager for turn tracking
    if service.live != nil {
        turnID, err := service.live.StartTurn(ctx, session, meta, input, runtimeOptions)
        if err != nil {
            return "", err
        }
        return turnID, nil
    }

    // Fallback to existing HTTP-based send (without turn tracking)
    // ... existing code ...
}
```

---

## Implementation Order

| Phase | Description | Effort | Dependencies |
|-------|-------------|--------|--------------|
| 1.1-1.3 | Define interfaces | 2h | None |
| 2.1-2.2 | Refactor CodexLiveManager | 4h | Phase 1 |
| 3.1-3.2 | Implement OpenCodeLiveSession | 6h | Phase 1, 2 |
| 4.1 | CompositeLiveManager | 3h | Phase 2, 3 |
| 5.1-5.3 | Wire into infrastructure | 2h | Phase 4 |
| Testing | Unit + integration tests | 4h | All phases |

**Total Estimated Effort**: 21 hours

---

## Testing Strategy

### Unit Tests

1. **Interface Compliance**
   - Verify `codexLiveSession` implements `TurnCapableSession`
   - Verify `openCodeLiveSession` implements `TurnCapableSession`

2. **OpenCodeLiveSession**
   - Test event stream handling
   - Test turn ID generation
   - Test turn completion publishing
   - Test interrupt handling

3. **CompositeLiveManager**
   - Test factory routing by provider
   - Test session caching
   - Test notifier propagation

### Integration Tests

1. **Guided Workflow with Kilocode**
   - Start workflow run
   - Verify step dispatch creates live session
   - Verify turn completion advances workflow
   - Verify checkpoint decisions work

2. **Multi-provider Scenarios**
   - Verify codex sessions still work
   - Verify opencode sessions work
   - Verify kilocode sessions work

---

## Rollout Plan

### Stage 1: Non-Breaking Interface Addition
- Add new interfaces (Phase 1)
- No behavior changes
- Deploy and verify stability

### Stage 2: Codex Refactor
- Refactor CodexLiveManager (Phase 2)
- Maintain backward compatibility
- Deploy and verify codex workflows still work

### Stage 3: OpenCode Support
- Add OpenCodeLiveSession (Phase 3)
- Add CompositeLiveManager (Phase 4)
- Deploy behind feature flag
- Enable for specific users

### Stage 4: Full Rollout
- Wire everywhere (Phase 5)
- Remove feature flag
- Monitor guided workflow success rates

---

## Success Criteria

1. ✅ Guided workflows complete successfully with kilocode provider
2. ✅ Guided workflows complete successfully with opencode provider
3. ✅ Existing codex guided workflows continue working
4. ✅ Turn completion events published within 1 second of session going idle
5. ✅ Interrupt functionality works for opencode/kilocode
6. ✅ All interface compliance tests pass
7. ✅ No regression in existing session management
