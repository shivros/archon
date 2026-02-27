package app

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"strings"
	"time"
)

type ChatTranscript struct {
	blocks            []ChatBlock
	maxBlocks         int
	activeAgentIndex  int
	pendingAgentBlock bool
	agentSegmentBreak bool
}

func NewChatTranscript(maxLines int) *ChatTranscript {
	return &ChatTranscript{
		maxBlocks:        maxLines,
		activeAgentIndex: -1,
	}
}

func (t *ChatTranscript) Reset() {
	if t == nil {
		return
	}
	t.blocks = nil
	t.activeAgentIndex = -1
	t.pendingAgentBlock = false
	t.agentSegmentBreak = false
}

func (t *ChatTranscript) SetBlocks(blocks []ChatBlock) {
	if t == nil {
		return
	}
	if len(blocks) == 0 {
		t.blocks = nil
	} else {
		t.blocks = append([]ChatBlock(nil), blocks...)
	}
	t.trim()
	t.activeAgentIndex = -1
	t.pendingAgentBlock = false
	t.agentSegmentBreak = false
}

func (t *ChatTranscript) Blocks() []ChatBlock {
	if t == nil {
		return nil
	}
	return t.blocks
}

func (t *ChatTranscript) AppendUserMessage(text string) int {
	return t.AppendUserMessageAt(text, time.Now().UTC())
}

func (t *ChatTranscript) AppendUserMessageAt(text string, createdAt time.Time) int {
	return t.appendUserMessageWithMetaAt(text, createdAt, "")
}

func (t *ChatTranscript) appendUserMessageWithMetaAt(text string, createdAt time.Time, turnID string) int {
	if t == nil {
		return -1
	}
	text = strings.TrimLeft(text, "\r\n")
	if strings.TrimSpace(text) == "" {
		return -1
	}
	if t.hasDuplicateMessageBlock(ChatRoleUser, text, turnID, createdAt) {
		return -1
	}
	headerIndex := len(t.blocks)
	t.blocks = append(t.blocks, ChatBlock{
		Role:      ChatRoleUser,
		Text:      text,
		Status:    ChatStatusNone,
		CreatedAt: createdAt,
		TurnID:    strings.TrimSpace(turnID),
	})
	t.trim()
	return headerIndex
}

func (t *ChatTranscript) StartAgentBlock() {
	t.StartAgentBlockAt(time.Now().UTC())
}

func (t *ChatTranscript) StartAgentBlockAt(createdAt time.Time) {
	if t == nil {
		return
	}
	if len(t.blocks) > 0 && t.blocks[len(t.blocks)-1].Role == ChatRoleAgent {
		t.activeAgentIndex = len(t.blocks) - 1
		t.pendingAgentBlock = true
		t.agentSegmentBreak = strings.TrimSpace(t.blocks[t.activeAgentIndex].Text) != ""
		return
	}
	t.blocks = append(t.blocks, ChatBlock{
		Role:      ChatRoleAgent,
		Text:      "",
		Status:    ChatStatusNone,
		CreatedAt: createdAt,
	})
	t.activeAgentIndex = len(t.blocks) - 1
	t.pendingAgentBlock = true
	t.agentSegmentBreak = false
	t.trim()
}

func (t *ChatTranscript) AppendAgentDelta(delta string) {
	t.AppendAgentDeltaAt(delta, time.Now().UTC())
}

func (t *ChatTranscript) AppendAgentDeltaAt(delta string, createdAt time.Time) {
	if t == nil {
		return
	}
	if t.activeAgentIndex < 0 || t.activeAgentIndex >= len(t.blocks) {
		if !t.pendingAgentBlock {
			t.StartAgentBlockAt(createdAt)
		}
	}
	if t.activeAgentIndex < 0 || t.activeAgentIndex >= len(t.blocks) {
		return
	}
	if t.agentSegmentBreak {
		t.blocks[t.activeAgentIndex].Text = concatAdjacentAgentText(t.blocks[t.activeAgentIndex].Text, delta)
		t.agentSegmentBreak = false
	} else {
		t.blocks[t.activeAgentIndex].Text += delta
	}
	if t.blocks[t.activeAgentIndex].CreatedAt.IsZero() && !createdAt.IsZero() {
		t.blocks[t.activeAgentIndex].CreatedAt = createdAt
	}
	t.trim()
}

func (t *ChatTranscript) FinishAgentBlock() {
	if t == nil {
		return
	}
	t.activeAgentIndex = -1
	t.pendingAgentBlock = false
	t.agentSegmentBreak = false
	t.trim()
}

func (t *ChatTranscript) MarkUserMessageFailed(headerIndex int) bool {
	if t == nil {
		return false
	}
	if headerIndex < 0 || headerIndex >= len(t.blocks) {
		return false
	}
	if t.blocks[headerIndex].Role != ChatRoleUser {
		return false
	}
	t.blocks[headerIndex].Status = ChatStatusFailed
	return true
}

func (t *ChatTranscript) MarkUserMessageSending(headerIndex int) bool {
	if t == nil {
		return false
	}
	if headerIndex < 0 || headerIndex >= len(t.blocks) {
		return false
	}
	if t.blocks[headerIndex].Role != ChatRoleUser {
		return false
	}
	t.blocks[headerIndex].Status = ChatStatusSending
	return true
}

func (t *ChatTranscript) MarkUserMessageSent(headerIndex int) bool {
	if t == nil {
		return false
	}
	if headerIndex < 0 || headerIndex >= len(t.blocks) {
		return false
	}
	if t.blocks[headerIndex].Role != ChatRoleUser {
		return false
	}
	t.blocks[headerIndex].Status = ChatStatusNone
	return true
}

func (t *ChatTranscript) UpsertReasoning(itemID, text string) bool {
	return t.UpsertReasoningAt(itemID, text, time.Now().UTC())
}

func (t *ChatTranscript) UpsertReasoningAt(itemID, text string, createdAt time.Time) bool {
	if t == nil {
		return false
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	itemID = strings.TrimSpace(itemID)
	if itemID == "" {
		t.appendBlockAt(ChatRoleReasoning, text, createdAt)
		return true
	}
	blockID := "reasoning:" + itemID
	for i := range t.blocks {
		if t.blocks[i].ID != blockID {
			continue
		}
		if strings.TrimSpace(t.blocks[i].Text) == text {
			return false
		}
		t.blocks[i].Text = text
		if t.blocks[i].CreatedAt.IsZero() && !createdAt.IsZero() {
			t.blocks[i].CreatedAt = createdAt
		}
		return true
	}
	t.blocks = append(t.blocks, ChatBlock{
		ID:        blockID,
		Role:      ChatRoleReasoning,
		Text:      text,
		Status:    ChatStatusNone,
		CreatedAt: createdAt,
		Collapsed: true,
	})
	t.trim()
	return true
}

func (t *ChatTranscript) UpsertApproval(req *ApprovalRequest) bool {
	if t == nil || req == nil || req.RequestID < 0 {
		return false
	}
	block := approvalRequestToBlock(req)
	for i := range t.blocks {
		if t.blocks[i].Role != ChatRoleApproval || t.blocks[i].RequestID != req.RequestID {
			continue
		}
		if t.blocks[i] == block {
			return false
		}
		t.blocks[i] = block
		return true
	}
	t.blocks = append(t.blocks, block)
	t.trim()
	return true
}

func (t *ChatTranscript) AppendItem(item map[string]any) {
	if t == nil || item == nil {
		return
	}
	createdAt := chatItemCreatedAt(item)
	turnID := itemTurnID(item)
	typ, _ := item["type"].(string)
	switch typ {
	case "log":
		if text := asString(item["text"]); text != "" {
			t.appendBlockAt(ChatRoleSystem, text, createdAt)
		}
	case "agentMessageDelta":
		if delta := asString(item["delta"]); delta != "" {
			t.AppendAgentDeltaAt(delta, createdAt)
		}
	case "agentMessageEnd":
		t.FinishAgentBlock()
	case "userMessage":
		if text := extractContentText(item["content"]); text != "" {
			t.appendUserMessageWithMetaAt(text, createdAt, turnID)
			return
		}
		if text := asString(item["text"]); text != "" {
			t.appendUserMessageWithMetaAt(text, createdAt, turnID)
		}
	case "agentMessage":
		if text := asString(item["text"]); text != "" {
			t.appendBlockWithMetaAt(ChatRoleAgent, text, createdAt, turnID)
			return
		}
		if text := extractContentText(item["content"]); text != "" {
			t.appendBlockWithMetaAt(ChatRoleAgent, text, createdAt, turnID)
		}
	case "assistant":
		if msg, ok := item["message"].(map[string]any); ok {
			if text := extractContentText(msg["content"]); text != "" {
				t.appendBlockWithMetaAt(ChatRoleAgent, text, createdAt, turnID)
				return
			}
		}
		if text := extractContentText(item["content"]); text != "" {
			t.appendBlockWithMetaAt(ChatRoleAgent, text, createdAt, turnID)
		}
	case "result":
		if text := asString(item["result"]); text != "" {
			t.appendBlockWithMetaAt(ChatRoleAgent, text, createdAt, turnID)
			return
		}
		if result, ok := item["result"].(map[string]any); ok {
			if text := asString(result["result"]); text != "" {
				t.appendBlockWithMetaAt(ChatRoleAgent, text, createdAt, turnID)
				return
			}
		}
	case "commandExecution":
		cmd := extractCommand(item["command"])
		status := asString(item["status"])
		lines := []string{"Command"}
		if cmd != "" {
			lines = append(lines, "", cmd)
		}
		if status != "" {
			lines = append(lines, "", "Status: "+status)
		}
		t.appendBlockAt(ChatRoleSystem, strings.Join(lines, "\n"), createdAt)
	case "fileChange":
		paths := extractChangePaths(item["changes"])
		if len(paths) > 0 {
			t.appendBlockAt(ChatRoleSystem, "File change\n\n"+strings.Join(paths, ", "), createdAt)
		}
	case "enteredReviewMode":
		if text := asString(item["review"]); text != "" {
			t.appendBlockAt(ChatRoleSystem, "Review started\n\n"+text, createdAt)
		}
	case "exitedReviewMode":
		if text := asString(item["review"]); text != "" {
			t.appendBlockAt(ChatRoleSystem, "Review completed\n\n"+text, createdAt)
		}
	case "reasoning":
		if text := reasoningText(item); text != "" {
			t.UpsertReasoningAt(asString(item["id"]), text, createdAt)
		}
	case "system":
		// Internal metadata (init, session info, etc.) — not shown to users.
		return
	default:
		if typ != "" {
			if data, err := json.Marshal(item); err == nil {
				t.appendBlockAt(ChatRoleSystem, fmt.Sprintf("%s: %s", typ, string(data)), createdAt)
				return
			}
		}
		if data, err := json.Marshal(item); err == nil {
			t.appendBlockAt(ChatRoleSystem, string(data), createdAt)
		}
	}
}

func reasoningText(item map[string]any) string {
	if item == nil {
		return ""
	}
	summary := extractStringList(item["summary"])
	if len(summary) > 0 {
		var lines []string
		for _, entry := range summary {
			for _, line := range strings.Split(entry, "\n") {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				lines = append(lines, "- "+line)
			}
		}
		return strings.Join(lines, "\n")
	}
	if text := strings.TrimLeft(extractContentText(item["content"]), "\r\n"); text != "" {
		return text
	}
	return ""
}

func (t *ChatTranscript) appendBlock(role ChatRole, text string) {
	t.appendBlockAt(role, text, time.Now().UTC())
}

func (t *ChatTranscript) appendBlockAt(role ChatRole, text string, createdAt time.Time) {
	t.appendBlockWithMetaAt(role, text, createdAt, "")
}

func (t *ChatTranscript) appendBlockWithMetaAt(role ChatRole, text string, createdAt time.Time, turnID string) {
	if t == nil || strings.TrimSpace(text) == "" {
		return
	}
	turnID = strings.TrimSpace(turnID)
	if role == ChatRoleAgent && t.hasDuplicateMessageBlock(role, text, turnID, createdAt) {
		return
	}
	if role == ChatRoleAgent && len(t.blocks) > 0 {
		last := len(t.blocks) - 1
		if t.blocks[last].Role == ChatRoleAgent {
			t.blocks[last].Text = concatAdjacentAgentText(t.blocks[last].Text, text)
			if strings.TrimSpace(t.blocks[last].TurnID) == "" {
				t.blocks[last].TurnID = turnID
			}
			if t.blocks[last].CreatedAt.IsZero() || (!createdAt.IsZero() && createdAt.Before(t.blocks[last].CreatedAt)) {
				t.blocks[last].CreatedAt = createdAt
			}
			return
		}
	}
	block := ChatBlock{
		ID:        makeChatBlockID(role, len(t.blocks), text),
		Role:      role,
		Text:      text,
		Status:    ChatStatusNone,
		CreatedAt: createdAt,
		Collapsed: role == ChatRoleReasoning,
		TurnID:    turnID,
	}
	t.blocks = append(t.blocks, block)
	t.trim()
}

func (t *ChatTranscript) hasDuplicateMessageBlock(role ChatRole, text, turnID string, createdAt time.Time) bool {
	if t == nil {
		return false
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	turnID = strings.TrimSpace(turnID)
	for i := range t.blocks {
		block := t.blocks[i]
		if block.Role != role {
			continue
		}
		if strings.TrimSpace(block.Text) != text {
			continue
		}
		blockTurnID := strings.TrimSpace(block.TurnID)
		if turnID != "" && blockTurnID == turnID {
			return true
		}
		if !createdAt.IsZero() && !block.CreatedAt.IsZero() && block.CreatedAt.Equal(createdAt) {
			return true
		}
	}
	return false
}

func itemTurnID(item map[string]any) string {
	if item == nil {
		return ""
	}
	if turnID := strings.TrimSpace(asString(item["turn_id"])); turnID != "" {
		return turnID
	}
	if turnID := strings.TrimSpace(asString(item["turnID"])); turnID != "" {
		return turnID
	}
	if turnRaw, ok := item["turn"].(map[string]any); ok && turnRaw != nil {
		if turnID := strings.TrimSpace(asString(turnRaw["id"])); turnID != "" {
			return turnID
		}
	}
	if msg, ok := item["message"].(map[string]any); ok && msg != nil {
		if turnID := strings.TrimSpace(asString(msg["turn_id"])); turnID != "" {
			return turnID
		}
		if turnID := strings.TrimSpace(asString(msg["turnID"])); turnID != "" {
			return turnID
		}
	}
	return ""
}

func concatAdjacentAgentText(current, next string) string {
	if strings.TrimSpace(next) == "" {
		return current
	}
	if strings.TrimSpace(current) == "" {
		return next
	}
	return current + next
}

func makeChatBlockID(role ChatRole, index int, text string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(text))
	return fmt.Sprintf("%s-%d-%x", role, index, h.Sum64())
}

func (t *ChatTranscript) trim() {
	if t.maxBlocks <= 0 || len(t.blocks) <= t.maxBlocks {
		return
	}
	drop := len(t.blocks) - t.maxBlocks
	t.blocks = t.blocks[drop:]
	if t.activeAgentIndex >= 0 {
		t.activeAgentIndex -= drop
		if t.activeAgentIndex < 0 {
			t.activeAgentIndex = len(t.blocks) - 1
		}
	}
}
