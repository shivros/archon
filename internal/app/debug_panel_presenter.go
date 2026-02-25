package app

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	xansi "github.com/charmbracelet/x/ansi"
)

type DebugPanelDisplayPolicy struct {
	PreviewMaxLines   int
	WrapPadding       int
	TruncationHint    string
	EmptyPayloadLabel string
}

func DefaultDebugPanelDisplayPolicy() DebugPanelDisplayPolicy {
	return DebugPanelDisplayPolicy{
		PreviewMaxLines:   5,
		WrapPadding:       8,
		TruncationHint:    "... (truncated, use [Expand])",
		EmptyPayloadLabel: "(empty debug payload)",
	}
}

type defaultDebugPanelPresenter struct {
	policy DebugPanelDisplayPolicy
}

func NewDefaultDebugPanelPresenter(policy DebugPanelDisplayPolicy) debugPanelPresenter {
	if policy.PreviewMaxLines <= 0 {
		policy.PreviewMaxLines = DefaultDebugPanelDisplayPolicy().PreviewMaxLines
	}
	if strings.TrimSpace(policy.TruncationHint) == "" {
		policy.TruncationHint = DefaultDebugPanelDisplayPolicy().TruncationHint
	}
	if strings.TrimSpace(policy.EmptyPayloadLabel) == "" {
		policy.EmptyPayloadLabel = DefaultDebugPanelDisplayPolicy().EmptyPayloadLabel
	}
	return defaultDebugPanelPresenter{policy: policy}
}

func (p defaultDebugPanelPresenter) Present(entries []DebugStreamEntry, width int, state DebugPanelPresentationState) DebugPanelPresentation {
	if len(entries) == 0 {
		return DebugPanelPresentation{}
	}
	blocks := make([]ChatBlock, 0, len(entries))
	metaByID := make(map[string]ChatBlockMetaPresentation, len(entries))
	copyByID := make(map[string]string, len(entries))
	wrapWidth := p.wrapWidth(width)
	for i, entry := range entries {
		blockID := strings.TrimSpace(entry.ID)
		if blockID == "" {
			blockID = fmt.Sprintf("debug-entry-%d", i)
		}
		full := escapeMarkdown(strings.TrimRight(entry.Display, "\n"))
		if strings.TrimSpace(full) == "" {
			full = p.policy.EmptyPayloadLabel
		}
		preview, truncated := p.previewText(full, wrapWidth)
		expanded := state.ExpandedByID[blockID]
		text := preview
		if expanded || !truncated {
			text = full
		}
		controls := []ChatMetaControl{{ID: debugMetaControlCopy, Label: "[Copy]", Tone: ChatMetaControlToneCopy}}
		if truncated {
			toggleLabel := "[Expand]"
			if expanded {
				toggleLabel = "[Collapse]"
			}
			controls = append(controls, ChatMetaControl{ID: debugMetaControlToggle, Label: toggleLabel, Tone: ChatMetaControlToneCopy})
		}
		metaByID[blockID] = ChatBlockMetaPresentation{
			PrimaryLabel: debugPrimaryLabel(entry),
			Label:        "Debug Event",
			Controls:     controls,
		}
		blocks = append(blocks, ChatBlock{
			ID:        blockID,
			Role:      ChatRoleSystem,
			Text:      text,
			CreatedAt: parseDebugTimestamp(entry.TS),
		})
		copyByID[blockID] = strings.TrimRight(entry.Display, "\n")
		if strings.TrimSpace(copyByID[blockID]) == "" {
			copyByID[blockID] = strings.TrimRight(entry.Raw, "\n")
		}
	}
	return DebugPanelPresentation{
		Blocks:       blocks,
		MetaByID:     metaByID,
		CopyTextByID: copyByID,
	}
}

func (p defaultDebugPanelPresenter) wrapWidth(width int) int {
	if width <= 0 {
		return 1
	}
	wrap := width - p.policy.WrapPadding
	if wrap < 1 {
		return 1
	}
	return wrap
}

func (p defaultDebugPanelPresenter) previewText(full string, wrapWidth int) (string, bool) {
	full = strings.TrimRight(full, "\n")
	if full == "" || p.policy.PreviewMaxLines <= 0 {
		return full, false
	}
	if wrapWidth <= 0 {
		wrapWidth = 1
	}
	wrapped := xansi.Hardwrap(full, wrapWidth, true)
	lines := strings.Split(strings.TrimRight(wrapped, "\n"), "\n")
	if len(lines) <= p.policy.PreviewMaxLines {
		return full, false
	}
	preview := strings.Join(lines[:p.policy.PreviewMaxLines], "\n")
	return strings.TrimRight(preview, "\n") + "\n\n" + p.policy.TruncationHint, true
}

type defaultDebugPanelBlocksRenderer struct{}

func NewDefaultDebugPanelBlocksRenderer() debugPanelBlocksRenderer {
	return defaultDebugPanelBlocksRenderer{}
}

func (defaultDebugPanelBlocksRenderer) Render(blocks []ChatBlock, width int, metaByID map[string]ChatBlockMetaPresentation) (string, []renderedBlockSpan) {
	if len(blocks) == 0 {
		return "", nil
	}
	now := time.Now()
	return renderChatBlocksWithRendererAndContext(
		blocks,
		width,
		maxViewportLines,
		-1,
		defaultChatBlockRenderer{},
		chatRenderContext{TimestampMode: ChatTimestampModeRelative, Now: now, MetaByBlockID: metaByID},
	)
}

func debugPrimaryLabel(entry DebugStreamEntry) string {
	parts := make([]string, 0, 3)
	stream := strings.TrimSpace(entry.Stream)
	if stream == "" {
		stream = "debug"
	}
	parts = append(parts, strings.ToUpper(stream))
	if entry.Seq > 0 {
		parts = append(parts, "#"+strconv.FormatUint(entry.Seq, 10))
	}
	if ts := parseDebugTimestamp(entry.TS); !ts.IsZero() {
		parts = append(parts, ts.Local().Format("15:04:05.000"))
	}
	return strings.Join(parts, " • ")
}

func parseDebugTimestamp(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}
	}
	return parsed
}
