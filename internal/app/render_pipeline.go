package app

import (
	"encoding/binary"
	"hash/fnv"
	"strings"
	"sync"
	"time"

	xansi "github.com/charmbracelet/x/ansi"
)

const defaultBlockRenderCacheSize = 4096
const defaultRenderResultCacheSize = 128

type RenderRequest struct {
	Width              int
	MaxLines           int
	RawContent         string
	EscapeMarkdown     bool
	Blocks             []ChatBlock
	BlockMetaByID      map[string]ChatBlockMetaPresentation
	SelectedBlockIndex int
	TimestampMode      ChatTimestampMode
	TimestampNow       time.Time
}

type RenderResult struct {
	Text       string
	Lines      []string
	PlainLines []string
	Spans      []renderedBlockSpan
}

type RenderPipeline interface {
	Render(req RenderRequest) RenderResult
}

func WithRenderPipeline(pipeline RenderPipeline) ModelOption {
	return func(m *Model) {
		if m == nil || pipeline == nil {
			return
		}
		m.renderPipeline = pipeline
	}
}

type defaultRenderPipeline struct {
	blockRenderer chatBlockRenderer
	resultCache   *renderResultCache
}

func NewDefaultRenderPipeline() RenderPipeline {
	blockCache := newBlockRenderCache(defaultBlockRenderCacheSize)
	return &defaultRenderPipeline{
		blockRenderer: newCachedChatBlockRenderer(defaultChatBlockRenderer{}, blockCache),
		resultCache:   newRenderResultCache(defaultRenderResultCacheSize),
	}
}

func (p *defaultRenderPipeline) Render(req RenderRequest) RenderResult {
	if p == nil {
		return RenderResult{}
	}
	width := req.Width
	if width <= 0 {
		width = 80
	}

	if req.Blocks != nil {
		mode := normalizeChatTimestampMode(req.TimestampMode)
		now := req.TimestampNow
		if now.IsZero() {
			now = time.Now()
		}
		ctx := chatRenderContext{
			TimestampMode: mode,
			Now:           now,
			MetaByBlockID: req.BlockMetaByID,
		}
		key := hashRenderRequestBlocks(req.Blocks, req.BlockMetaByID, width, req.MaxLines, req.SelectedBlockIndex, mode, chatTimestampRenderBucket(mode, now))
		if cached, ok := p.resultCache.Get(key); ok {
			return cached
		}
		text, spans := renderChatBlocksWithRendererAndContext(req.Blocks, width, req.MaxLines, req.SelectedBlockIndex, p.blockRenderer, ctx)
		result := newRenderResult(text, spans)
		p.resultCache.Set(key, result)
		return result
	}

	key := hashRenderRequestRaw(req.RawContent, req.EscapeMarkdown, width)
	if cached, ok := p.resultCache.Get(key); ok {
		return cached
	}
	content := req.RawContent
	if req.EscapeMarkdown {
		content = escapeMarkdown(content)
	}
	text := renderMarkdown(content, width)
	result := newRenderResult(text, nil)
	p.resultCache.Set(key, result)
	return result
}

func newRenderResult(text string, spans []renderedBlockSpan) RenderResult {
	result := RenderResult{Text: text, Spans: spans}
	if text == "" {
		return result
	}
	lines := strings.Split(text, "\n")
	result.Lines = lines
	plain := make([]string, len(lines))
	for i, line := range lines {
		plain[i] = xansi.Strip(line)
	}
	result.PlainLines = plain
	return result
}

type renderResultCache struct {
	mu      sync.Mutex
	entries map[uint64]RenderResult
	order   []uint64
	maxSize int
}

func newRenderResultCache(maxSize int) *renderResultCache {
	if maxSize < 1 {
		maxSize = 1
	}
	return &renderResultCache{
		entries: map[uint64]RenderResult{},
		order:   make([]uint64, 0, maxSize),
		maxSize: maxSize,
	}
}

func (c *renderResultCache) Get(key uint64) (RenderResult, bool) {
	if c == nil {
		return RenderResult{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	result, ok := c.entries[key]
	if !ok {
		return RenderResult{}, false
	}
	return result, true
}

func (c *renderResultCache) Set(key uint64, value RenderResult) {
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

func hashRenderRequestBlocks(blocks []ChatBlock, blockMetaByID map[string]ChatBlockMetaPresentation, width, maxLines, selected int, mode ChatTimestampMode, relativeBucket int64) uint64 {
	hasher := fnv.New64a()
	writeHashInt(hasher, width)
	writeHashInt(hasher, maxLines)
	writeHashInt(hasher, selected)
	writeHashString(hasher, string(mode))
	writeHashInt64(hasher, relativeBucket)
	writeHashInt(hasher, len(blocks))
	var buf [8]byte
	for _, block := range blocks {
		binary.LittleEndian.PutUint64(buf[:], hashChatBlock(block))
		_, _ = hasher.Write(buf[:])
		if len(blockMetaByID) == 0 || strings.TrimSpace(block.ID) == "" {
			_, _ = hasher.Write([]byte{0})
			continue
		}
		meta, ok := blockMetaByID[strings.TrimSpace(block.ID)]
		if !ok {
			_, _ = hasher.Write([]byte{0})
			continue
		}
		_, _ = hasher.Write([]byte{1})
		binary.LittleEndian.PutUint64(buf[:], hashChatBlockMetaPresentation(meta))
		_, _ = hasher.Write(buf[:])
	}
	return hasher.Sum64()
}

func hashRenderRequestRaw(raw string, escaped bool, width int) uint64 {
	hasher := fnv.New64a()
	writeHashInt(hasher, width)
	if escaped {
		_, _ = hasher.Write([]byte{1})
	} else {
		_, _ = hasher.Write([]byte{0})
	}
	writeHashString(hasher, raw)
	return hasher.Sum64()
}
