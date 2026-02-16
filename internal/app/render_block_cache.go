package app

import (
	"encoding/binary"
	"hash"
	"hash/fnv"
	"strings"
	"sync"
)

type chatBlockRenderer interface {
	RenderChatBlock(block ChatBlock, width int, selected bool) renderedChatBlock
}

type defaultChatBlockRenderer struct{}

func (defaultChatBlockRenderer) RenderChatBlock(block ChatBlock, width int, selected bool) renderedChatBlock {
	return renderChatBlock(block, width, selected)
}

type blockRenderKey struct {
	blockHash uint64
	width     int
	selected  bool
}

type blockRenderCache struct {
	mu      sync.Mutex
	entries map[blockRenderKey]renderedChatBlock
	order   []blockRenderKey
	maxSize int
}

func newBlockRenderCache(maxSize int) *blockRenderCache {
	if maxSize < 1 {
		maxSize = 1
	}
	return &blockRenderCache{
		entries: map[blockRenderKey]renderedChatBlock{},
		order:   make([]blockRenderKey, 0, maxSize),
		maxSize: maxSize,
	}
}

func (c *blockRenderCache) Get(key blockRenderKey) (renderedChatBlock, bool) {
	if c == nil {
		return renderedChatBlock{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	val, ok := c.entries[key]
	if !ok {
		return renderedChatBlock{}, false
	}
	return val, true
}

func (c *blockRenderCache) Set(key blockRenderKey, value renderedChatBlock) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.entries[key]; !exists {
		c.order = append(c.order, key)
	}
	c.entries[key] = value
	for len(c.order) > c.maxSize {
		evict := c.order[0]
		c.order = c.order[1:]
		delete(c.entries, evict)
	}
}

type cachedChatBlockRenderer struct {
	next  chatBlockRenderer
	cache *blockRenderCache
}

func newCachedChatBlockRenderer(next chatBlockRenderer, cache *blockRenderCache) chatBlockRenderer {
	if next == nil {
		next = defaultChatBlockRenderer{}
	}
	return &cachedChatBlockRenderer{next: next, cache: cache}
}

func (r *cachedChatBlockRenderer) RenderChatBlock(block ChatBlock, width int, selected bool) renderedChatBlock {
	if r == nil || r.next == nil {
		return renderedChatBlock{}
	}
	key := blockRenderKey{
		blockHash: hashChatBlock(block),
		width:     width,
		selected:  selected,
	}
	if cached, ok := r.cache.Get(key); ok {
		return cached
	}
	rendered := r.next.RenderChatBlock(block, width, selected)
	r.cache.Set(key, rendered)
	return rendered
}

func hashChatBlock(block ChatBlock) uint64 {
	hasher := fnv.New64a()
	writeHashString(hasher, block.ID)
	writeHashString(hasher, string(block.Role))
	writeHashString(hasher, block.Text)
	writeHashString(hasher, string(block.Status))
	writeHashString(hasher, block.SessionID)
	writeHashBool(hasher, block.Collapsed)
	writeHashInt(hasher, block.RequestID)
	return hasher.Sum64()
}

func writeHashString(hasher hash.Hash64, value string) {
	if hasher == nil {
		return
	}
	value = strings.TrimSpace(value)
	_, _ = hasher.Write([]byte(value))
	_, _ = hasher.Write([]byte{0})
}

func writeHashInt(hasher hash.Hash64, value int) {
	if hasher == nil {
		return
	}
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, uint64(value))
	_, _ = hasher.Write(buf)
}

func writeHashBool(hasher hash.Hash64, value bool) {
	if hasher == nil {
		return
	}
	if value {
		_, _ = hasher.Write([]byte{1})
		return
	}
	_, _ = hasher.Write([]byte{0})
}
