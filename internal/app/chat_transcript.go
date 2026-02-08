package app

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"strings"
)

type ChatTranscript struct {
	blocks            []ChatBlock
	maxBlocks         int
	activeAgentIndex  int
	pendingAgentBlock bool
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
}

func (t *ChatTranscript) Blocks() []ChatBlock {
	if t == nil {
		return nil
	}
	return t.blocks
}

func (t *ChatTranscript) AppendUserMessage(text string) int {
	if t == nil || strings.TrimSpace(text) == "" {
		return -1
	}
	headerIndex := len(t.blocks)
	t.blocks = append(t.blocks, ChatBlock{
		Role:   ChatRoleUser,
		Text:   text,
		Status: ChatStatusNone,
	})
	t.trim()
	return headerIndex
}

func (t *ChatTranscript) StartAgentBlock() {
	if t == nil {
		return
	}
	t.blocks = append(t.blocks, ChatBlock{
		Role:   ChatRoleAgent,
		Text:   "",
		Status: ChatStatusNone,
	})
	t.activeAgentIndex = len(t.blocks) - 1
	t.pendingAgentBlock = true
	t.trim()
}

func (t *ChatTranscript) AppendAgentDelta(delta string) {
	if t == nil {
		return
	}
	if t.activeAgentIndex < 0 || t.activeAgentIndex >= len(t.blocks) {
		if !t.pendingAgentBlock {
			t.StartAgentBlock()
		}
	}
	if t.activeAgentIndex < 0 || t.activeAgentIndex >= len(t.blocks) {
		return
	}
	t.blocks[t.activeAgentIndex].Text += delta
	t.trim()
}

func (t *ChatTranscript) FinishAgentBlock() {
	if t == nil {
		return
	}
	t.activeAgentIndex = -1
	t.pendingAgentBlock = false
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

func (t *ChatTranscript) AppendItem(item map[string]any) {
	if t == nil || item == nil {
		return
	}
	typ, _ := item["type"].(string)
	switch typ {
	case "log":
		if text := asString(item["text"]); text != "" {
			t.appendBlock(ChatRoleSystem, text)
		}
	case "agentMessageDelta":
		if delta := asString(item["delta"]); delta != "" {
			t.AppendAgentDelta(delta)
		}
	case "agentMessageEnd":
		t.FinishAgentBlock()
	case "userMessage":
		if text := extractContentText(item["content"]); text != "" {
			t.AppendUserMessage(text)
			return
		}
		if text := asString(item["text"]); text != "" {
			t.AppendUserMessage(text)
		}
	case "agentMessage":
		if text := asString(item["text"]); text != "" {
			t.appendBlock(ChatRoleAgent, text)
			return
		}
		if text := extractContentText(item["content"]); text != "" {
			t.appendBlock(ChatRoleAgent, text)
		}
	case "assistant":
		if msg, ok := item["message"].(map[string]any); ok {
			if text := extractContentText(msg["content"]); text != "" {
				t.appendBlock(ChatRoleAgent, text)
				return
			}
		}
		if text := extractContentText(item["content"]); text != "" {
			t.appendBlock(ChatRoleAgent, text)
		}
	case "result":
		if text := asString(item["result"]); text != "" {
			t.appendBlock(ChatRoleAgent, text)
			return
		}
		if result, ok := item["result"].(map[string]any); ok {
			if text := asString(result["result"]); text != "" {
				t.appendBlock(ChatRoleAgent, text)
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
		t.appendBlock(ChatRoleSystem, strings.Join(lines, "\n"))
	case "fileChange":
		paths := extractChangePaths(item["changes"])
		if len(paths) > 0 {
			t.appendBlock(ChatRoleSystem, "File change\n\n"+strings.Join(paths, ", "))
		}
	case "enteredReviewMode":
		if text := asString(item["review"]); text != "" {
			t.appendBlock(ChatRoleSystem, "Review started\n\n"+text)
		}
	case "exitedReviewMode":
		if text := asString(item["review"]); text != "" {
			t.appendBlock(ChatRoleSystem, "Review completed\n\n"+text)
		}
	case "reasoning":
		summary := extractStringList(item["summary"])
		if len(summary) > 0 {
			lines := []string{"Reasoning (summary)", ""}
			for _, entry := range summary {
				for _, line := range strings.Split(entry, "\n") {
					line = strings.TrimSpace(line)
					if line == "" {
						continue
					}
					lines = append(lines, "- "+line)
				}
			}
			t.appendBlock(ChatRoleReasoning, strings.Join(lines, "\n"))
			return
		}
		if text := extractContentText(item["content"]); text != "" {
			t.appendBlock(ChatRoleReasoning, "Reasoning\n\n"+text)
		}
	default:
		if typ != "" {
			if data, err := json.Marshal(item); err == nil {
				t.appendBlock(ChatRoleSystem, fmt.Sprintf("%s: %s", typ, string(data)))
				return
			}
		}
		if data, err := json.Marshal(item); err == nil {
			t.appendBlock(ChatRoleSystem, string(data))
		}
	}
}

func (t *ChatTranscript) appendBlock(role ChatRole, text string) {
	if t == nil || strings.TrimSpace(text) == "" {
		return
	}
	block := ChatBlock{
		ID:        makeChatBlockID(role, len(t.blocks), text),
		Role:      role,
		Text:      text,
		Status:    ChatStatusNone,
		Collapsed: role == ChatRoleReasoning,
	}
	t.blocks = append(t.blocks, block)
	t.trim()
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
