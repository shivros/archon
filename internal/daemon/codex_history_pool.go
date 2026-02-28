package daemon

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"control/internal/logging"
)

const (
	defaultCodexHistoryPoolIdleTTL    = 90 * time.Second
	defaultCodexHistoryPoolMaxClients = 6
)

type CodexHistoryPool interface {
	ReadThread(ctx context.Context, cwd, codexHome, threadID string) (*codexThread, error)
	Close()
}

type codexHistoryPool struct {
	mu         sync.Mutex
	clients    map[string]*pooledCodexHistoryClient
	idleTTL    time.Duration
	maxClients int
	logger     logging.Logger
	startFn    func(ctx context.Context, cwd, codexHome string, logger logging.Logger) (codexHistoryClient, error)
}

type pooledCodexHistoryClient struct {
	key       string
	cwd       string
	codexHome string
	client    codexHistoryClient
	lastUsed  time.Time
	inUse     int
}

type codexHistoryClient interface {
	ReadThread(ctx context.Context, threadID string) (*codexThread, error)
	ResumeThread(ctx context.Context, threadID string) error
	Close()
}

func NewCodexHistoryPool(logger logging.Logger) CodexHistoryPool {
	if logger == nil {
		logger = logging.Nop()
	}
	return &codexHistoryPool{
		clients:    map[string]*pooledCodexHistoryClient{},
		idleTTL:    defaultCodexHistoryPoolIdleTTL,
		maxClients: defaultCodexHistoryPoolMaxClients,
		logger:     logger,
		startFn: func(ctx context.Context, cwd, codexHome string, logger logging.Logger) (codexHistoryClient, error) {
			return startCodexAppServer(ctx, cwd, codexHome, logger)
		},
	}
}

func (p *codexHistoryPool) ReadThread(ctx context.Context, cwd, codexHome, threadID string) (*codexThread, error) {
	if p == nil {
		return nil, errors.New("codex history pool is required")
	}
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return nil, errors.New("thread id is required")
	}
	entry, release, err := p.acquire(ctx, cwd, codexHome)
	if err != nil {
		return nil, err
	}
	thread, err := p.readThreadWithRecovery(ctx, entry.client, threadID)
	release()
	if err == nil {
		return thread, nil
	}
	if !isClosedPipeError(err) {
		return nil, err
	}
	p.invalidate(entry.key, entry.client)
	entry, release, reacquireErr := p.acquire(ctx, cwd, codexHome)
	if reacquireErr != nil {
		return nil, reacquireErr
	}
	thread, err = p.readThreadWithRecovery(ctx, entry.client, threadID)
	release()
	return thread, err
}

const codexHistoryRetryAttempts = 3
const codexHistoryRetryDelay = 300 * time.Millisecond

func (p *codexHistoryPool) readThreadWithRecovery(ctx context.Context, client codexHistoryClient, threadID string) (*codexThread, error) {
	if client == nil {
		return nil, errors.New("codex history client is required")
	}
	thread, err := p.tryReadWithResume(ctx, client, threadID)
	if err == nil {
		return thread, nil
	}
	if !isCodexHistoryResumeRequiredError(err) {
		return nil, err
	}
	// For brand-new threads, both ReadThread and ResumeThread may fail
	// because the pooled app-server hasn't indexed the thread yet.
	// Retry with short backoff.
	for attempt := 1; attempt <= codexHistoryRetryAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(codexHistoryRetryDelay):
		}
		p.logger.Debug("codex_history_retry",
			logging.F("thread_id", threadID),
			logging.F("attempt", attempt),
			logging.F("previous_error", err.Error()),
		)
		thread, err = p.tryReadWithResume(ctx, client, threadID)
		if err == nil {
			return thread, nil
		}
		if !isCodexHistoryResumeRequiredError(err) {
			return nil, err
		}
	}
	return nil, err
}

func (p *codexHistoryPool) tryReadWithResume(ctx context.Context, client codexHistoryClient, threadID string) (*codexThread, error) {
	thread, err := client.ReadThread(ctx, threadID)
	if err == nil {
		return thread, nil
	}
	if !isCodexHistoryResumeRequiredError(err) {
		return nil, err
	}
	resumeErr := client.ResumeThread(ctx, threadID)
	if resumeErr != nil {
		if isCodexHistoryResumeRequiredError(resumeErr) {
			return nil, resumeErr
		}
		p.logger.Warn("codex_history_resume_failed",
			logging.F("thread_id", threadID),
			logging.F("read_error", err.Error()),
			logging.F("resume_error", resumeErr.Error()),
		)
		return nil, fmt.Errorf("thread read error: %v; resume error: %w", err, resumeErr)
	}
	return client.ReadThread(ctx, threadID)
}

func isCodexHistoryResumeRequiredError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(text, "thread not loaded") || strings.Contains(text, "no rollout found for thread id")
}

func (p *codexHistoryPool) Close() {
	if p == nil {
		return
	}
	p.mu.Lock()
	entries := make([]*pooledCodexHistoryClient, 0, len(p.clients))
	for key, entry := range p.clients {
		delete(p.clients, key)
		if entry != nil {
			entries = append(entries, entry)
		}
	}
	p.mu.Unlock()
	for _, entry := range entries {
		if entry.client != nil {
			entry.client.Close()
		}
	}
}

func (p *codexHistoryPool) acquire(ctx context.Context, cwd, codexHome string) (*pooledCodexHistoryClient, func(), error) {
	key := codexHistoryPoolKey(cwd, codexHome)
	if key == "" {
		return nil, nil, errors.New("workspace key is required")
	}

	now := time.Now().UTC()
	var stale []*pooledCodexHistoryClient

	p.mu.Lock()
	stale = append(stale, p.evictIdleLocked(now)...)
	if entry := p.clients[key]; entry != nil && entry.client != nil {
		entry.inUse++
		entry.lastUsed = now
		release := p.releaseFunc(entry)
		p.mu.Unlock()
		p.closeEntries(stale)
		return entry, release, nil
	}
	p.mu.Unlock()
	p.closeEntries(stale)

	startFn := p.startFn
	if startFn == nil {
		startFn = func(ctx context.Context, cwd, codexHome string, logger logging.Logger) (codexHistoryClient, error) {
			return startCodexAppServer(ctx, cwd, codexHome, logger)
		}
	}
	client, err := startFn(ctx, cwd, codexHome, p.logger)
	if err != nil {
		return nil, nil, err
	}

	now = time.Now().UTC()
	var closeAfter []*pooledCodexHistoryClient
	p.mu.Lock()
	if existing := p.clients[key]; existing != nil && existing.client != nil {
		existing.inUse++
		existing.lastUsed = now
		release := p.releaseFunc(existing)
		p.mu.Unlock()
		client.Close()
		return existing, release, nil
	}
	entry := &pooledCodexHistoryClient{
		key:       key,
		cwd:       strings.TrimSpace(cwd),
		codexHome: strings.TrimSpace(codexHome),
		client:    client,
		lastUsed:  now,
		inUse:     1,
	}
	p.clients[key] = entry
	closeAfter = append(closeAfter, p.evictExcessLocked()...)
	release := p.releaseFunc(entry)
	p.mu.Unlock()
	p.closeEntries(closeAfter)
	return entry, release, nil
}

func (p *codexHistoryPool) invalidate(key string, client codexHistoryClient) {
	if p == nil || client == nil {
		return
	}
	p.mu.Lock()
	entry := p.clients[key]
	if entry != nil && entry.client == client && entry.inUse == 0 {
		delete(p.clients, key)
	}
	p.mu.Unlock()
	client.Close()
}

func (p *codexHistoryPool) releaseFunc(entry *pooledCodexHistoryClient) func() {
	return func() {
		if p == nil || entry == nil {
			return
		}
		p.mu.Lock()
		if current := p.clients[entry.key]; current != nil && current == entry {
			if current.inUse > 0 {
				current.inUse--
			}
			current.lastUsed = time.Now().UTC()
		}
		p.mu.Unlock()
	}
}

func (p *codexHistoryPool) evictIdleLocked(now time.Time) []*pooledCodexHistoryClient {
	if p.idleTTL <= 0 {
		return nil
	}
	closed := make([]*pooledCodexHistoryClient, 0)
	for key, entry := range p.clients {
		if entry == nil || entry.client == nil || entry.inUse > 0 {
			continue
		}
		if now.Sub(entry.lastUsed) < p.idleTTL {
			continue
		}
		delete(p.clients, key)
		closed = append(closed, entry)
	}
	return closed
}

func (p *codexHistoryPool) evictExcessLocked() []*pooledCodexHistoryClient {
	if p.maxClients <= 0 || len(p.clients) <= p.maxClients {
		return nil
	}
	idle := make([]*pooledCodexHistoryClient, 0, len(p.clients))
	for _, entry := range p.clients {
		if entry == nil || entry.client == nil || entry.inUse > 0 {
			continue
		}
		idle = append(idle, entry)
	}
	if len(idle) == 0 {
		return nil
	}
	sort.Slice(idle, func(i, j int) bool {
		return idle[i].lastUsed.Before(idle[j].lastUsed)
	})
	remove := len(p.clients) - p.maxClients
	if remove <= 0 {
		return nil
	}
	closed := make([]*pooledCodexHistoryClient, 0, remove)
	for i := 0; i < len(idle) && remove > 0; i++ {
		entry := idle[i]
		if entry == nil {
			continue
		}
		if current := p.clients[entry.key]; current == nil || current != entry || current.inUse > 0 {
			continue
		}
		delete(p.clients, entry.key)
		closed = append(closed, entry)
		remove--
	}
	return closed
}

func (p *codexHistoryPool) closeEntries(entries []*pooledCodexHistoryClient) {
	for _, entry := range entries {
		if entry != nil && entry.client != nil {
			entry.client.Close()
		}
	}
}

func codexHistoryPoolKey(cwd, codexHome string) string {
	if home := strings.TrimSpace(codexHome); home != "" {
		return home
	}
	return strings.TrimSpace(cwd)
}
