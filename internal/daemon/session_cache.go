package daemon

import "sync"

type SessionCache interface {
	Get(sessionID string) TurnCapableSession
	Set(sessionID string, session TurnCapableSession)
	Delete(sessionID string) TurnCapableSession
}

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
