# SOLID Refactoring Plan: Live Session Architecture

## Overview

This plan addresses the SOLID violations identified in the audit, ordered by priority and dependency. Each phase builds on the previous, ensuring the system remains functional throughout.

---

## Phase 1: NotifiableSession Interface (OCP Fix)

**Problem:** Type assertion in `CompositeLiveManager.ensure()` violates Open/Closed Principle.

**Current Code:**
```go
// composite_live_manager.go:130-132
if ols, ok := ls.(*openCodeLiveSession); ok && m.notifier != nil {
    ols.notifier = m.notifier
}
```

### 1.1 Define NotifiableSession Interface

**File:** `internal/daemon/live_session.go`

```go
// NotifiableSession can receive a notification publisher for emitting events.
type NotifiableSession interface {
    LiveSession
    SetNotificationPublisher(notifier NotificationPublisher)
}
```

### 1.2 Add SetNotificationPublisher to openCodeLiveSession

**File:** `internal/daemon/opencode_live_session.go`

```go
func (s *openCodeLiveSession) SetNotificationPublisher(notifier NotificationPublisher) {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.notifier = notifier
}
```

Add interface compliance:
```go
var (
    _ LiveSession        = (*openCodeLiveSession)(nil)
    _ TurnCapableSession = (*openCodeLiveSession)(nil)
    _ NotifiableSession  = (*openCodeLiveSession)(nil)
)
```

### 1.3 Add SetNotificationPublisher to codexLiveSession

**File:** `internal/daemon/codex_live.go`

```go
func (s *codexLiveSession) SetNotificationPublisher(notifier NotificationPublisher) {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.notifier = notifier
}
```

Add interface compliance:
```go
var (
    _ LiveSession            = (*codexLiveSession)(nil)
    _ TurnCapableSession     = (*codexLiveSession)(nil)
    _ ApprovalCapableSession = (*codexLiveSession)(nil)
    _ NotifiableSession      = (*codexLiveSession)(nil)
)
```

### 1.4 Update CompositeLiveManager.ensure()

**File:** `internal/daemon/composite_live_manager.go`

Replace:
```go
if ols, ok := ls.(*openCodeLiveSession); ok && m.notifier != nil {
    ols.notifier = m.notifier
}
```

With:
```go
if ns, ok := ls.(NotifiableSession); ok && m.notifier != nil {
    ns.SetNotificationPublisher(m.notifier)
}
```

### 1.5 Update CompositeLiveManager.SetNotificationPublisher()

**File:** `internal/daemon/composite_live_manager.go`

Replace:
```go
func (m *CompositeLiveManager) SetNotificationPublisher(notifier NotificationPublisher) {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.notifier = notifier
    for _, s := range m.sessions {
        if ls, ok := s.(*openCodeLiveSession); ok && ls != nil {
            ls.notifier = notifier
        }
    }
}
```

With:
```go
func (m *CompositeLiveManager) SetNotificationPublisher(notifier NotificationPublisher) {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.notifier = notifier
    for _, s := range m.sessions {
        if ns, ok := s.(NotifiableSession); ok {
            ns.SetNotificationPublisher(notifier)
        }
    }
}
```

**Effort:** 1 hour

---

## Phase 2: Segregate LiveManager Interface (ISP Fix)

**Problem:** `LiveManager` interface is too large with 5 methods.

### 2.1 Define Segregated Interfaces

**File:** `internal/daemon/live_manager.go`

```go
// TurnStarter initiates turns for workflow dispatch.
type TurnStarter interface {
    StartTurn(ctx context.Context, session *types.Session, meta *types.SessionMeta, input []map[string]any, opts *types.SessionRuntimeOptions) (string, error)
}

// EventStreamer provides event subscription for UI.
type EventStreamer interface {
    Subscribe(session *types.Session, meta *types.SessionMeta) (<-chan types.CodexEvent, func(), error)
}

// ApprovalResponder handles approval responses.
type ApprovalResponder interface {
    Respond(ctx context.Context, session *types.Session, meta *types.SessionMeta, requestID int, result map[string]any) error
}

// TurnInterrupter cancels active turns.
type TurnInterrupter interface {
    Interrupt(ctx context.Context, session *types.Session, meta *types.SessionMeta) error
}

// NotificationReceiver accepts notification publishers.
type NotificationReceiver interface {
    SetNotificationPublisher(notifier NotificationPublisher)
}

// LiveManager combines all capabilities for full-featured consumers.
type LiveManager interface {
    TurnStarter
    EventStreamer
    ApprovalResponder
    TurnInterrupter
    NotificationReceiver
}
```

### 2.2 Update Consumers to Use Specific Interfaces

**File:** `internal/daemon/session_conversation_adapters.go`

Change:
```go
func (a openCodeConversationAdapter) SendMessage(..., service *SessionService) {
    if service.liveManager != nil {
        turnID, err := service.liveManager.StartTurn(...)
    }
}
```

To use specific interface:
```go
func (a openCodeConversationAdapter) SendMessage(..., service *SessionService) {
    if service.turnStarter != nil {
        turnID, err := service.turnStarter.StartTurn(...)
    }
}
```

### 2.3 Update SessionService

**File:** `internal/daemon/session_service.go`

```go
type SessionService struct {
    manager      *SessionManager
    stores       *Stores
    live         *CodexLiveManager      // Keep for backward compatibility temporarily
    turnStarter  TurnStarter            // Specific interface for turn starting
    eventStreamer EventStreamer         // Specific interface for event streaming
    logger       logging.Logger
    // ...
}
```

Add options:
```go
func WithTurnStarter(starter TurnStarter) SessionServiceOption {
    return func(s *SessionService) {
        if s == nil || starter == nil {
            return
        }
        s.turnStarter = starter
    }
}

func WithEventStreamer(streamer EventStreamer) SessionServiceOption {
    return func(s *SessionService) {
        if s == nil || streamer == nil {
            return
        }
        s.eventStreamer = streamer
    }
}
```

**Effort:** 2 hours

---

## Phase 3: Remove Concrete CodexLiveManager Dependency (DIP Fix)

**Problem:** `SessionService` depends on concrete `*CodexLiveManager`.

### 3.1 Audit All `live` Field Usage

Find all usages of `service.live` in `SessionService`:
- `codexConversationAdapter.SendMessage` - uses `service.live.StartTurn()`
- Other locations that need codex-specific behavior

### 3.2 Update codexConversationAdapter

**File:** `internal/daemon/session_conversation_adapters.go`

Change:
```go
func (codexConversationAdapter) SendMessage(ctx context.Context, service *SessionService, ...) (string, error) {
    turnID, err := service.live.StartTurn(ctx, session, meta, codexHome, input, runtimeOptions)
}
```

To use `turnStarter` interface:
```go
func (codexConversationAdapter) SendMessage(ctx context.Context, service *SessionService, ...) (string, error) {
    if service.turnStarter != nil {
        turnID, err := service.turnStarter.StartTurn(ctx, session, meta, input, runtimeOptions)
    }
}
```

### 3.3 Remove `live` Field from SessionService

**File:** `internal/daemon/session_service.go`

```go
type SessionService struct {
    manager      *SessionManager
    stores       *Stores
    // live         *CodexLiveManager  // REMOVE
    turnStarter  TurnStarter
    eventStreamer EventStreamer
    logger       logging.Logger
    // ...
}
```

### 3.4 Update NewSessionService

**File:** `internal/daemon/session_service.go`

Remove `live` parameter:
```go
func NewSessionService(manager *SessionManager, stores *Stores, logger logging.Logger, opts ...SessionServiceOption) *SessionService {
    // ...
}
```

Update all callers to use options:
```go
// api.go
return NewSessionService(a.Manager, a.Stores, a.Logger,
    WithTurnStarter(a.LiveManager),
    WithEventStreamer(a.LiveManager),
    // ... other options
)
```

### 3.5 Update Function Signature

Change:
```go
func NewSessionService(manager *SessionManager, stores *Stores, live *CodexLiveManager, logger logging.Logger, opts ...SessionServiceOption) *SessionService
```

To:
```go
func NewSessionService(manager *SessionManager, stores *Stores, logger logging.Logger, opts ...SessionServiceOption) *SessionService
```

This is a breaking change - update all callers:
- `api.go:184`
- `guided_workflows_bridge.go:388`
- `workflow_run_session_interrupt_service.go:48`
- All test files

**Effort:** 3 hours

---

## Phase 4: Extract TurnCompletionNotifier (SRP Fix)

**Problem:** `openCodeLiveSession` handles both event streaming and notification publishing.

### 4.1 Define TurnCompletionNotifier Interface

**File:** `internal/daemon/turn_notifier.go` (new file)

```go
package daemon

import "context"

// TurnCompletionNotifier publishes turn completion events.
type TurnCompletionNotifier interface {
    NotifyTurnCompleted(ctx context.Context, sessionID, turnID, provider string, meta *SessionMeta)
}
```

### 4.2 Implement DefaultNotifier

**File:** `internal/daemon/turn_notifier.go`

```go
type DefaultTurnCompletionNotifier struct {
    notifier NotificationPublisher
    stores   *Stores
}

func NewTurnCompletionNotifier(notifier NotificationPublisher, stores *Stores) *DefaultTurnCompletionNotifier {
    return &DefaultTurnCompletionNotifier{
        notifier: notifier,
        stores:   stores,
    }
}

func (n *DefaultTurnCompletionNotifier) NotifyTurnCompleted(ctx context.Context, sessionID, turnID, provider string, meta *SessionMeta) {
    if n.notifier == nil {
        return
    }

    event := types.NotificationEvent{
        Trigger:    types.NotificationTriggerTurnCompleted,
        OccurredAt: time.Now().UTC().Format(time.RFC3339Nano),
        SessionID:  sessionID,
        TurnID:     turnID,
        Provider:   provider,
        Source:     "live_session_event",
    }

    if meta != nil {
        event.WorkspaceID = meta.WorkspaceID
        event.WorktreeID = meta.WorktreeID
    } else if n.stores != nil && n.stores.SessionMeta != nil {
        if m, ok, _ := n.stores.SessionMeta.Get(ctx, sessionID); ok && m != nil {
            event.WorkspaceID = m.WorkspaceID
            event.WorktreeID = m.WorktreeID
        }
    }

    n.notifier.Publish(event)
}
```

### 4.3 Update openCodeLiveSession

**File:** `internal/daemon/opencode_live_session.go`

```go
type openCodeLiveSession struct {
    mu           sync.Mutex
    sessionID    string
    providerName string
    providerID   string
    directory    string
    client       *openCodeClient
    events       <-chan types.CodexEvent
    cancelEvents func()
    hub          *codexSubscriberHub
    notifier     TurnCompletionNotifier  // Changed from NotificationPublisher
    activeTurn   string
    closed       bool
}
```

Update `publishTurnCompleted`:
```go
func (s *openCodeLiveSession) publishTurnCompleted(turnID string) {
    if s.notifier == nil {
        return
    }
    s.notifier.NotifyTurnCompleted(context.Background(), s.sessionID, turnID, s.providerName, nil)
}
```

### 4.4 Update Factory

**File:** `internal/daemon/opencode_live_session.go`

```go
type openCodeLiveSessionFactory struct {
    providerName string
    stores       *Stores
    logger       logging.Logger
    notifier     TurnCompletionNotifier  // Add
}

func (f *openCodeLiveSessionFactory) CreateTurnCapable(...) (TurnCapableSession, error) {
    // ...
    ls := &openCodeLiveSession{
        sessionID:    session.ID,
        providerName: f.providerName,
        providerID:   providerID,
        directory:    session.Cwd,
        client:       client,
        events:       events,
        cancelEvents: cancel,
        hub:          newCodexSubscriberHub(),
        notifier:     f.notifier,  // Inject
    }
    // ...
}
```

**Effort:** 2 hours

---

## Phase 5: Extract ApprovalStorage (SRP Fix)

**Problem:** `openCodeLiveSession` handles approval persistence.

### 5.1 Define ApprovalStorage Interface

**File:** `internal/daemon/approval_storage.go` (new file)

```go
package daemon

import (
    "context"
    "time"

    "control/internal/types"
)

// ApprovalStorage persists approval requests.
type ApprovalStorage interface {
    StoreApproval(ctx context.Context, sessionID string, requestID int, method string, params json.RawMessage) error
}

// NopApprovalStorage is a no-op implementation.
type NopApprovalStorage struct{}

func (NopApprovalStorage) StoreApproval(_ context.Context, _ string, _ int, _ string, _ json.RawMessage) error {
    return nil
}
```

### 5.2 Implement DefaultApprovalStorage

**File:** `internal/daemon/approval_storage.go`

```go
type StoreApprovalStorage struct {
    stores *Stores
}

func NewStoreApprovalStorage(stores *Stores) *StoreApprovalStorage {
    return &StoreApprovalStorage{stores: stores}
}

func (s *StoreApprovalStorage) StoreApproval(ctx context.Context, sessionID string, requestID int, method string, params json.RawMessage) error {
    if s.stores == nil || s.stores.Approvals == nil {
        return nil
    }

    approval := &types.Approval{
        SessionID: sessionID,
        RequestID: requestID,
        Method:    method,
        Params:    params,
        CreatedAt: time.Now().UTC(),
    }
    _, err := s.stores.Approvals.Upsert(ctx, approval)
    return err
}
```

### 5.3 Update openCodeLiveSession

**File:** `internal/daemon/opencode_live_session.go`

```go
type openCodeLiveSession struct {
    mu             sync.Mutex
    sessionID      string
    providerName   string
    providerID     string
    directory      string
    client         *openCodeClient
    events         <-chan types.CodexEvent
    cancelEvents   func()
    hub            *codexSubscriberHub
    turnNotifier   TurnCompletionNotifier
    approvalStore  ApprovalStorage  // New
    activeTurn     string
    closed         bool
}
```

Update `storeApproval`:
```go
func (s *openCodeLiveSession) storeApproval(event types.CodexEvent) {
    if s.approvalStore == nil || event.ID == nil {
        return
    }
    _ = s.approvalStore.StoreApproval(context.Background(), s.sessionID, *event.ID, event.Method, event.Params)
}
```

**Effort:** 1.5 hours

---

## Phase 6: Extract SessionCache (SRP Fix)

**Problem:** `CompositeLiveManager` mixes caching with routing.

### 6.1 Define SessionCache Interface

**File:** `internal/daemon/session_cache.go` (new file)

```go
package daemon

// SessionCache manages live session instances.
type SessionCache interface {
    Get(sessionID string) TurnCapableSession
    Set(sessionID string, session TurnCapableSession)
    Delete(sessionID string) TurnCapableSession
}
```

### 6.2 Implement DefaultSessionCache

**File:** `internal/daemon/session_cache.go`

```go
type MemorySessionCache struct {
    mu       sync.Mutex
    sessions map[string]TurnCapableSession
}

func NewMemorySessionCache() *MemorySessionCache {
    return &MemorySessionCache{
        sessions: make(map[string]TurnCapableSession),
    }
}

func (c *MemorySessionCache) Get(sessionID string) TurnCapableSession {
    c.mu.Lock()
    defer c.mu.Unlock()
    return c.sessions[sessionID]
}

func (c *MemorySessionCache) Set(sessionID string, session TurnCapableSession) {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.sessions[sessionID] = session
}

func (c *MemorySessionCache) Delete(sessionID string) TurnCapableSession {
    c.mu.Lock()
    defer c.mu.Unlock()
    session := c.sessions[sessionID]
    delete(c.sessions, sessionID)
    return session
}
```

### 6.3 Update CompositeLiveManager

**File:** `internal/daemon/composite_live_manager.go`

```go
type CompositeLiveManager struct {
    factories map[string]TurnCapableSessionFactory
    cache     SessionCache              // Extracted
    logger    logging.Logger
    notifier  NotificationPublisher
}

func NewCompositeLiveManager(logger logging.Logger, cache SessionCache, factories ...TurnCapableSessionFactory) *CompositeLiveManager {
    if cache == nil {
        cache = NewMemorySessionCache()
    }
    // ...
}

func (m *CompositeLiveManager) ensure(ctx context.Context, session *types.Session, meta *types.SessionMeta) (TurnCapableSession, error) {
    // Check cache
    if ls := m.cache.Get(session.ID); ls != nil {
        return ls, nil
    }

    // Create via factory
    provider := providers.Normalize(session.Provider)
    factory := m.factories[provider]
    if factory == nil {
        return nil, errors.New("provider does not support live sessions")
    }

    ls, err := factory.CreateTurnCapable(ctx, session, meta)
    if err != nil {
        return nil, err
    }

    // Configure notification
    if ns, ok := ls.(NotifiableSession); ok && m.notifier != nil {
        ns.SetNotificationPublisher(m.notifier)
    }

    // Store in cache
    m.cache.Set(session.ID, ls)
    return ls, nil
}
```

**Effort:** 1.5 hours

---

## Phase 7: Inject Client into Factory (DIP Fix)

**Problem:** Factory creates concrete `openCodeClient`.

### 7.1 Define OpenCodeClient Interface

**File:** `internal/daemon/opencode_client_interface.go` (new file)

```go
package daemon

import (
    "context"
    "control/internal/types"
)

// OpenCodeClient defines operations needed for live sessions.
type OpenCodeClient interface {
    SubscribeSessionEvents(ctx context.Context, sessionID, directory string) (<-chan types.CodexEvent, func(), error)
    Prompt(ctx context.Context, sessionID, text string, opts *types.SessionRuntimeOptions, directory string) (string, error)
    AbortSession(ctx context.Context, sessionID, directory string) error
}
```

### 7.2 Ensure openCodeClient Implements Interface

Add interface compliance:
```go
var _ OpenCodeClient = (*openCodeClient)(nil)
```

### 7.3 Update Factory to Accept Client

**File:** `internal/daemon/opencode_live_session.go`

```go
type openCodeLiveSessionFactory struct {
    providerName string
    logger       logging.Logger
    turnNotifier TurnCompletionNotifier
    approvalStore ApprovalStorage
    client       OpenCodeClient  // Interface, not concrete
}

func newOpenCodeLiveSessionFactory(providerName string, logger logging.Logger, client OpenCodeClient) *openCodeLiveSessionFactory {
    return &openCodeLiveSessionFactory{
        providerName: providerName,
        logger:       logger,
        client:       client,
    }
}

func (f *openCodeLiveSessionFactory) CreateTurnCapable(ctx context.Context, session *types.Session, meta *types.SessionMeta) (TurnCapableSession, error) {
    // Use injected client instead of creating one
    events, cancel, err := f.client.SubscribeSessionEvents(ctx, providerID, session.Cwd)
    // ...
}
```

### 7.4 Update daemon.go Initialization

**File:** `internal/daemon/daemon.go`

```go
opencodeClient, _ := newOpenCodeClient(resolveOpenCodeClientConfig("opencode", coreCfg))
kilocodeClient, _ := newOpenCodeClient(resolveOpenCodeClientConfig("kilocode", coreCfg))

turnNotifier := NewTurnCompletionNotifier(eventPublisher, d.stores)
approvalStore := NewStoreApprovalStorage(d.stores)

compositeLive := NewCompositeLiveManager(
    d.logger,
    NewMemorySessionCache(),
    newCodexLiveSessionFactory(liveCodex),
    newOpenCodeLiveSessionFactory("opencode", d.logger, opencodeClient).
        WithTurnNotifier(turnNotifier).
        WithApprovalStorage(approvalStore),
    newOpenCodeLiveSessionFactory("kilocode", d.logger, kilocodeClient).
        WithTurnNotifier(turnNotifier).
        WithApprovalStorage(approvalStore),
)
```

**Effort:** 2 hours

---

## Implementation Order

| Phase | Description | Effort | Dependencies |
|-------|-------------|--------|--------------|
| 1 | NotifiableSession interface (OCP) | 1h | None |
| 2 | Segregate LiveManager (ISP) | 2h | Phase 1 |
| 3 | Remove concrete dependency (DIP) | 3h | Phase 2 |
| 4 | Extract TurnCompletionNotifier (SRP) | 2h | Phase 1 |
| 5 | Extract ApprovalStorage (SRP) | 1.5h | Phase 4 |
| 6 | Extract SessionCache (SRP) | 1.5h | Phase 1 |
| 7 | Inject client into factory (DIP) | 2h | Phase 4, 5 |

**Total Effort:** 13 hours

---

## Testing Strategy

### Unit Tests

1. **NotifiableSession** - Verify notification publisher is set and used
2. **Segregated interfaces** - Ensure consumers compile with specific interfaces
3. **TurnCompletionNotifier** - Verify correct event structure
4. **ApprovalStorage** - Verify correct persistence
5. **SessionCache** - Verify get/set/delete operations

### Integration Tests

1. **End-to-end guided workflow** - Verify kilocode still works after refactoring
2. **Notification flow** - Verify turn completion notifications reach subscribers
3. **Approval flow** - Verify approvals are stored correctly

### Regression Tests

1. Run all existing tests after each phase
2. Verify codex workflows continue to work
3. Verify opencode/kilocode workflows continue to work

---

## Rollback Plan

Each phase is designed to be independently reversible:

1. **Phase 1-2** - Interface additions are non-breaking; can be removed if needed
2. **Phase 3** - Keep `live` field temporarily during migration; remove after verification
3. **Phase 4-6** - New interfaces can be unwired if issues arise
4. **Phase 7** - Client injection is optional; factory can create client if not provided

---

## Success Criteria

1. ✅ All type assertions removed from `CompositeLiveManager`
2. ✅ `SessionService` has no concrete provider dependencies
3. ✅ Each interface has ≤3 methods (ISP compliance)
4. ✅ Each struct has single reason to change (SRP compliance)
5. ✅ All existing tests pass
6. ✅ Guided workflows work for codex, opencode, and kilocode
