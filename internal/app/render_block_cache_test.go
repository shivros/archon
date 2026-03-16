package app

import (
	"testing"
	"time"
)

type countingChatBlockRenderer struct {
	calls int
}

func (r *countingChatBlockRenderer) RenderChatBlock(block ChatBlock, width int, selected bool, _ chatRenderContext) renderedChatBlock {
	r.calls++
	line := block.Text
	if selected {
		line += " [selected]"
	}
	return renderedChatBlock{Lines: []string{line}}
}

func TestCachedChatBlockRendererCachesByBlockWidthAndSelection(t *testing.T) {
	counter := &countingChatBlockRenderer{}
	cache := newBlockRenderCache(32)
	renderer := newCachedChatBlockRenderer(counter, cache)

	blocks := []ChatBlock{
		{ID: "1", Role: ChatRoleUser, Text: "hello"},
		{ID: "2", Role: ChatRoleAgent, Text: "world"},
	}

	_, _ = renderChatBlocksWithRenderer(blocks, 80, 2000, -1, renderer)
	if counter.calls != 2 {
		t.Fatalf("expected 2 renderer calls, got %d", counter.calls)
	}

	_, _ = renderChatBlocksWithRenderer(blocks, 80, 2000, -1, renderer)
	if counter.calls != 2 {
		t.Fatalf("expected cache hit on identical render, got %d calls", counter.calls)
	}

	_, _ = renderChatBlocksWithRenderer(blocks, 80, 2000, 0, renderer)
	if counter.calls != 3 {
		t.Fatalf("expected selected-state change to rerender one block, got %d calls", counter.calls)
	}

	_, _ = renderChatBlocksWithRenderer(blocks, 120, 2000, 0, renderer)
	if counter.calls != 5 {
		t.Fatalf("expected width change to rerender both blocks, got %d calls", counter.calls)
	}
}

func TestCachedChatBlockRendererInvalidatesOnThemeChange(t *testing.T) {
	counter := &countingChatBlockRenderer{}
	cache := newBlockRenderCache(32)
	renderer := newCachedChatBlockRenderer(counter, cache)
	blocks := []ChatBlock{{ID: "1", Role: ChatRoleUser, Text: "hello"}}
	now := time.Unix(1700000000, 0)

	_, _ = renderChatBlocksWithRendererAndContext(
		blocks,
		80,
		2000,
		-1,
		-1,
		-1,
		renderer,
		chatRenderContext{
			TimestampMode: ChatTimestampModeRelative,
			Now:           now,
			ThemeID:       "default",
		},
	)
	if counter.calls != 1 {
		t.Fatalf("expected first render call, got %d", counter.calls)
	}

	_, _ = renderChatBlocksWithRendererAndContext(
		blocks,
		80,
		2000,
		-1,
		-1,
		-1,
		renderer,
		chatRenderContext{
			TimestampMode: ChatTimestampModeRelative,
			Now:           now,
			ThemeID:       "monokai",
		},
	)
	if counter.calls != 2 {
		t.Fatalf("expected theme change to invalidate block cache, got %d calls", counter.calls)
	}
}
