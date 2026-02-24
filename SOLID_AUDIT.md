# SOLID Compliance Audit

## Summary

| Principle | Status | Score |
|-----------|--------|-------|
| Single Responsibility | Partial | 6/10 |
| Open/Closed | Partial | 7/10 |
| Liskov Substitution | Good | 8/10 |
| Interface Segregation | Good | 8/10 |
| Dependency Inversion | Partial | 6/10 |

**Overall: 7/10** - Acceptable but with notable violations.

---

## 1. Single Responsibility Principle (SRP)

> A class should have one, and only one, reason to change.

### Violations

#### 1.1 `openCodeLiveSession` Has Multiple Responsibilities

**Location:** `internal/daemon/opencode_live_session.go:12-26`

```go
type openCodeLiveSession struct {
    // Session identity (OK)
    sessionID    string
    providerName string
    providerID   string

    // Event streaming (OK - core responsibility)
    client       *openCodeClient
    events       <-chan types.CodexEvent
    cancelEvents func()
    hub          *codexSubscriberHub

    // Turn management (OK - core responsibility)
    activeTurn   string

    // Notification publishing (VIOLATION - separate concern)
    notifier     NotificationPublisher

    // Data persistence (VIOLATION - separate concern)
    stores       *Stores
}
```

**Problem:** The session struct handles:
1. Event stream management (core)
2. Turn state tracking (core)
3. Notification publishing (should be delegated)
4. Approval persistence (should be delegated)

**Impact:** Changes to notification format or storage schema require modifying the session.

#### 1.2 `CompositeLiveManager` Mixes Caching with Routing

**Location:** `internal/daemon/composite_live_manager.go:14-21`

```go
type CompositeLiveManager struct {
    factories map[string]TurnCapableSessionFactory
    sessions  map[string]TurnCapableSession  // Caching responsibility
    stores    *Stores                         // Data access responsibility
    notifier  NotificationPublisher          // Notification responsibility
}
```

**Problem:** Combines:
- Factory routing (core)
- Session caching (could be separate)
- Notification propagation (could be separate)

### Compliant Areas

✅ **Factory pattern** - `TurnCapableSessionFactory` has single responsibility of creating sessions.

✅ **Interface separation** - `LiveSession`, `TurnCapableSession`, `ApprovalCapableSession` each define focused contracts.

---

## 2. Open/Closed Principle (OCP)

> Software entities should be open for extension, but closed for modification.

### Violations

#### 2.1 Type Assertion for Notifier Propagation

**Location:** `internal/daemon/composite_live_manager.go:130-132`

```go
func (m *CompositeLiveManager) ensure(...) (TurnCapableSession, error) {
    // ...
    if ols, ok := ls.(*openCodeLiveSession); ok && m.notifier != nil {
        ols.notifier = m.notifier
    }
    // ...
}
```

**Problem:** Adding a new provider (e.g., `geminiLiveSession`) requires modifying this method to handle the new type.

**Fix:** Add `SetNotificationPublisher` to the interface:

```go
type NotifiableSession interface {
    LiveSession
    SetNotificationPublisher(notifier NotificationPublisher)
}

// Then in ensure:
if ns, ok := ls.(NotifiableSession); ok && m.notifier != nil {
    ns.SetNotificationPublisher(m.notifier)
}
```

#### 2.2 Provider-Specific Logic in Factory

**Location:** `internal/daemon/composite_live_manager.go:171-172`

```go
func (f *codexLiveSessionFactory) CreateTurnCapable(...) (TurnCapableSession, error) {
    if session.Provider != "codex" {
        return nil, errors.New("provider does not support codex live sessions")
    }
    // ...
}
```

**Problem:** The factory checks the provider name internally. This logic should be handled by the factory registration/routing, not inside the factory.

### Compliant Areas

✅ **Factory registration** - Adding new providers requires only registering a new factory, not modifying existing code:

```go
compositeLive := NewCompositeLiveManager(
    d.stores, d.logger,
    newCodexLiveSessionFactory(liveCodex),
    newOpenCodeLiveSessionFactory("opencode", d.stores, d.logger),
    newOpenCodeLiveSessionFactory("kilocode", d.stores, d.logger),
    // New providers added here without modifying CompositeLiveManager
)
```

✅ **Interface-based design** - `LiveManager` interface allows consumers to use the abstraction without knowing implementation details.

---

## 3. Liskov Substitution Principle (LSP)

> Subtypes must be substitutable for their base types.

### Compliant Areas

✅ **Interface implementations are substitutable:**

```go
var (
    _ LiveSession        = (*openCodeLiveSession)(nil)
    _ TurnCapableSession = (*openCodeLiveSession)(nil)
)

var (
    _ LiveSession        = (*codexLiveSession)(nil)
    _ TurnCapableSession = (*codexLiveSession)(nil)
)
```

✅ **Behavior is consistent across implementations:**

Both `openCodeLiveSession.StartTurn()` and `codexLiveSession.StartTurn()`:
- Return a turn ID string
- Return an error on failure
- Have the same signature

### Minor Concerns

⚠️ **Error message inconsistency:**

```go
// codexLiveSession.Interrupt
func (s *codexLiveSession) Interrupt(ctx context.Context) error {
    if turnID == "" {
        return errors.New("no active turn")  // Specific error
    }
    // ...
}

// openCodeLiveSession.Interrupt
func (s *openCodeLiveSession) Interrupt(ctx context.Context) error {
    return s.client.AbortSession(ctx, s.providerID, s.directory)  // Different behavior
}
```

**Impact:** Clients expecting "no active turn" error will not receive it from opencode. However, this is acceptable as the contract only promises an error on failure.

---

## 4. Interface Segregation Principle (ISP)

> Many client-specific interfaces are better than one general-purpose interface.

### Compliant Areas

✅ **Session interfaces are properly segregated:**

```go
// Base interface - minimal
type LiveSession interface {
    Events() <-chan types.CodexEvent
    Close()
    SessionID() string
}

// Extended only when needed
type TurnCapableSession interface {
    LiveSession
    StartTurn(...) (string, error)
    Interrupt(ctx context.Context) error
    ActiveTurnID() string
}

// Separate capability
type ApprovalCapableSession interface {
    LiveSession
    Respond(ctx context.Context, requestID int, result map[string]any) error
}
```

This allows:
- Clients that only need events don't depend on turn methods
- Clients that only need approvals don't depend on turn methods
- Sessions can implement only what they support

### Violations

#### 4.1 `LiveManager` Interface is Too Large

**Location:** `internal/daemon/live_manager.go:9-15`

```go
type LiveManager interface {
    StartTurn(...) (string, error)      // Workflow dispatch needs this
    Subscribe(...) (<-chan, func(), error) // UI streaming needs this
    Respond(...) error                   // Approval handling needs this
    Interrupt(...) error                 // Session control needs this
    SetNotificationPublisher(...)        // Infrastructure needs this
}
```

**Problem:** Clients depending on `LiveManager` see methods they don't need:
- `guidedWorkflowPromptDispatcher` only needs `StartTurn`
- UI event streaming only needs `Subscribe`
- Approval handlers only need `Respond`

**Fix:** Segregate into focused interfaces:

```go
type TurnStarter interface {
    StartTurn(ctx context.Context, session *types.Session, meta *types.SessionMeta, input []map[string]any, opts *types.SessionRuntimeOptions) (string, error)
}

type EventStreamer interface {
    Subscribe(session *types.Session, meta *types.SessionMeta) (<-chan types.CodexEvent, func(), error)
}

type ApprovalResponder interface {
    Respond(ctx context.Context, session *types.Session, meta *types.SessionMeta, requestID int, result map[string]any) error
}

type TurnInterrupter interface {
    Interrupt(ctx context.Context, session *types.Session, meta *types.SessionMeta) error
}
```

Then consumers depend only on what they need:

```go
// In openCodeConversationAdapter
func (a openCodeConversationAdapter) SendMessage(..., starter TurnStarter) {
    turnID, err := starter.StartTurn(...)
}
```

---

## 5. Dependency Inversion Principle (DIP)

> Depend on abstractions, not concretions.

### Violations

#### 5.1 `SessionService` Depends on Concrete Type

**Location:** `internal/daemon/session_service.go:17-30`

```go
type SessionService struct {
    live        *CodexLiveManager  // CONCRETE - VIOLATION
    liveManager LiveManager        // ABSTRACT - GOOD
    // ...
}
```

**Problem:** `SessionService` has both the concrete `*CodexLiveManager` and the abstract `LiveManager`. The concrete dependency exists for backward compatibility with existing code that calls `live.StartTurn()` with the `codexHome` parameter.

**Impact:**
- Harder to test (must mock concrete type)
- Tied to codex implementation details

**Fix:** Remove `live` field entirely and use only `liveManager`:

```go
type SessionService struct {
    liveManager LiveManager  // Only abstract dependency
    // ...
}
```

#### 5.2 Factory Creates Concrete Client

**Location:** `internal/daemon/opencode_live_session.go:188`

```go
func (f *openCodeLiveSessionFactory) CreateTurnCapable(...) (TurnCapableSession, error) {
    client, err := newOpenCodeClient(resolveOpenCodeClientConfig(f.providerName, loadCoreConfigOrDefault()))
    // ...
}
```

**Problem:** Factory directly instantiates `openCodeClient` rather than receiving it as a dependency.

**Impact:**
- Cannot inject mock client for testing
- Tied to specific client implementation

**Fix:** Accept client as a dependency:

```go
type openCodeLiveSessionFactory struct {
    providerName string
    stores       *Stores
    logger       logging.Logger
    client       OpenCodeClient  // Interface, not concrete
}

type OpenCodeClient interface {
    SubscribeSessionEvents(ctx context.Context, sessionID, directory string) (<-chan types.CodexEvent, func(), error)
    Prompt(ctx context.Context, sessionID, text string, opts *types.SessionRuntimeOptions, directory string) (string, error)
    AbortSession(ctx context.Context, sessionID, directory string) error
}
```

#### 5.3 `codexLiveSessionFactory` Wraps Concrete Manager

**Location:** `internal/daemon/composite_live_manager.go:152-158`

```go
type codexLiveSessionFactory struct {
    manager *CodexLiveManager  // CONCRETE - VIOLATION
}

func (f *codexLiveSessionFactory) CreateTurnCapable(...) (TurnCapableSession, error) {
    ls, err := f.manager.ensure(ctx, session, meta, codexHome)
    // ...
}
```

**Problem:** Factory depends on concrete `*CodexLiveManager` rather than an abstraction.

### Compliant Areas

✅ **Factory interface is abstract:**

```go
type TurnCapableSessionFactory interface {
    CreateTurnCapable(ctx context.Context, session *types.Session, meta *types.SessionMeta) (TurnCapableSession, error)
    ProviderName() string
}
```

✅ **Session interface is abstract:**

```go
type TurnCapableSession interface {
    LiveSession
    StartTurn(ctx context.Context, input []map[string]any, opts *types.SessionRuntimeOptions) (string, error)
    Interrupt(ctx context.Context) error
    ActiveTurnID() string
}
```

---

## Recommended Fixes (Priority Order)

### High Priority

1. **Remove concrete `live` field from `SessionService`** (DIP violation)
   - Use only `liveManager` interface
   - Update all callers

2. **Add `NotifiableSession` interface** (OCP violation)
   ```go
   type NotifiableSession interface {
       LiveSession
       SetNotificationPublisher(notifier NotificationPublisher)
   }
   ```
   - Remove type assertion in `CompositeLiveManager.ensure()`

### Medium Priority

3. **Segregate `LiveManager` interface** (ISP violation)
   - Create `TurnStarter`, `EventStreamer`, `ApprovalResponder` interfaces
   - Update consumers to depend on specific interfaces

4. **Extract notification logic from `openCodeLiveSession`** (SRP violation)
   - Create `TurnCompletionNotifier` interface
   - Delegate publishing to separate component

### Low Priority

5. **Accept client as factory dependency** (DIP violation)
   - Define `OpenCodeClient` interface
   - Allow injection for testing

6. **Extract caching from `CompositeLiveManager`** (SRP violation)
   - Create `SessionCache` interface
   - Separate session lifecycle from routing

---

## Conclusion

The implementation achieves the primary goal of supporting opencode/kilocode for guided workflows. The architecture is reasonably well-structured with proper use of interfaces for the core abstractions.

However, there are notable SOLID violations that should be addressed:

1. **DIP violations** in `SessionService` and factories create testing challenges
2. **SRP violations** in session structs make them harder to maintain
3. **OCP violation** with type assertions will cause issues when adding new providers

The most critical fix is removing the concrete `*CodexLiveManager` dependency from `SessionService` and addressing the type assertion in `CompositeLiveManager.ensure()`.
