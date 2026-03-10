package daemon

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"control/internal/config"
	"control/internal/types"
)

const claudeApprovalSyncTailLines = 512

type claudeApprovalSyncProvider struct{}

func (p *claudeApprovalSyncProvider) Provider() string {
	return "claude"
}

func (p *claudeApprovalSyncProvider) SyncSessionApprovals(_ context.Context, session *types.Session, _ *types.SessionMeta) (*ApprovalSyncResult, error) {
	if session == nil || strings.TrimSpace(session.ID) == "" {
		return nil, nil
	}

	baseDir, err := config.SessionsDir()
	if err != nil {
		return nil, err
	}
	sessionDir := filepath.Join(baseDir, strings.TrimSpace(session.ID))
	debugEvents, authoritative, err := readClaudeApprovalDebugEvents(filepath.Join(sessionDir, "debug.jsonl"))
	if err != nil {
		return nil, err
	}
	if len(debugEvents) == 0 {
		return &ApprovalSyncResult{Authoritative: authoritative}, nil
	}

	lastUserMessageAt, err := readLastClaudeUserMessageAt(filepath.Join(sessionDir, "items.jsonl"))
	if err != nil {
		return nil, err
	}

	approval := extractClaudePlanApproval(debugEvents, session.ID, lastUserMessageAt)
	if approval == nil {
		return &ApprovalSyncResult{Authoritative: authoritative}, nil
	}
	return &ApprovalSyncResult{
		Approvals:     []*types.Approval{approval},
		Authoritative: true,
	}, nil
}

type claudeDebugApprovalEvent struct {
	At      time.Time
	Payload map[string]any
}

func readClaudeApprovalDebugEvents(path string) ([]claudeDebugApprovalEvent, bool, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	rawLines, _, err := tailLines(path, claudeApprovalSyncTailLines)
	if err != nil {
		return nil, false, err
	}
	events := make([]claudeDebugApprovalEvent, 0, len(rawLines))
	for _, line := range rawLines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var event types.DebugEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(event.Provider), "claude") {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(event.Stream), "provider_stdout_raw") {
			continue
		}
		payload := map[string]any{}
		if err := json.Unmarshal([]byte(event.Chunk), &payload); err != nil {
			continue
		}
		var at time.Time
		if ts := strings.TrimSpace(event.TS); ts != "" {
			if parsed, err := time.Parse(time.RFC3339Nano, ts); err == nil {
				at = parsed.UTC()
			}
		}
		events = append(events, claudeDebugApprovalEvent{
			At:      at,
			Payload: payload,
		})
	}
	return events, true, nil
}

func readLastClaudeUserMessageAt(path string) (time.Time, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return time.Time{}, nil
		}
		return time.Time{}, err
	}
	rawLines, _, err := tailLines(path, claudeApprovalSyncTailLines)
	if err != nil {
		return time.Time{}, err
	}
	latest := time.Time{}
	for _, line := range rawLines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		payload := map[string]any{}
		if err := json.Unmarshal([]byte(line), &payload); err != nil {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(asString(payload["type"])), "userMessage") {
			continue
		}
		createdAt := parseApprovalTime(payload["created_at"])
		if createdAt.After(latest) {
			latest = createdAt
		}
	}
	return latest, nil
}

func extractClaudePlanApproval(events []claudeDebugApprovalEvent, sessionID string, lastUserMessageAt time.Time) *types.Approval {
	if len(events) == 0 {
		return nil
	}
	var latest *types.Approval
	for _, event := range events {
		payload := event.Payload
		if !strings.EqualFold(strings.TrimSpace(asString(payload["type"])), "assistant") {
			continue
		}
		message, _ := payload["message"].(map[string]any)
		content, _ := message["content"].([]any)
		for _, rawBlock := range content {
			block, _ := rawBlock.(map[string]any)
			if block == nil {
				continue
			}
			if !strings.EqualFold(strings.TrimSpace(asString(block["type"])), "tool_use") {
				continue
			}
			if !strings.EqualFold(strings.TrimSpace(asString(block["name"])), "ExitPlanMode") {
				continue
			}
			if !lastUserMessageAt.IsZero() && event.At.Before(lastUserMessageAt) {
				continue
			}
			input, _ := block["input"].(map[string]any)
			params := claudeExitPlanApprovalParams(block, input)
			paramsRaw, _ := json.Marshal(params)
			createdAt := event.At
			if createdAt.IsZero() {
				createdAt = time.Now().UTC()
			}
			latest = &types.Approval{
				SessionID: sessionID,
				RequestID: claudeApprovalRequestID(asString(block["id"]), asString(block["name"])),
				Method:    types.ApprovalMethodClaudeExitPlanMode,
				Params:    paramsRaw,
				CreatedAt: createdAt,
			}
		}
	}
	return latest
}

func claudeExitPlanApprovalParams(block map[string]any, input map[string]any) map[string]any {
	params := map[string]any{
		"tool_use_id": asString(block["id"]),
		"tool_name":   asString(block["name"]),
		"message":     "Claude is waiting for plan approval before implementation can continue.",
	}
	if plan := strings.TrimSpace(asString(input["plan"])); plan != "" {
		params["plan"] = plan
	}
	if title := strings.TrimSpace(claudePlanTitle(asString(input["plan"]))); title != "" {
		params["title"] = title
	}
	if prompts := claudeAllowedPrompts(input["allowedPrompts"]); len(prompts) > 0 {
		params["allowed_prompts"] = prompts
	}
	return params
}

func claudeAllowedPrompts(raw any) []string {
	items, _ := raw.([]any)
	if len(items) == 0 {
		return nil
	}
	out := make([]string, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		typed, _ := item.(map[string]any)
		if typed == nil {
			continue
		}
		tool := strings.TrimSpace(asString(typed["tool"]))
		prompt := strings.TrimSpace(asString(typed["prompt"]))
		text := prompt
		if tool != "" && prompt != "" {
			text = tool + ": " + prompt
		} else if tool != "" {
			text = tool
		}
		if text == "" {
			continue
		}
		if _, ok := seen[text]; ok {
			continue
		}
		seen[text] = struct{}{}
		out = append(out, text)
	}
	return out
}

func claudePlanTitle(plan string) string {
	plan = strings.TrimSpace(plan)
	if plan == "" {
		return ""
	}
	for _, line := range strings.Split(plan, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = strings.TrimLeft(line, "#")
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

func claudeApprovalRequestID(toolUseID string, fallback string) int {
	key := strings.TrimSpace(toolUseID)
	if key == "" {
		key = strings.TrimSpace(fallback)
	}
	if key == "" {
		key = "claude-exit-plan-mode"
	}
	sum := sha256.Sum256([]byte(key))
	value := binary.BigEndian.Uint32(sum[:4]) & 0x7fffffff
	if value == 0 {
		return 1
	}
	return int(value)
}

type claudeConversationApprover struct {
	providerName string
}

func (a claudeConversationApprover) Provider() string {
	return a.providerName
}

func (a claudeConversationApprover) Approve(
	ctx context.Context,
	deps approvalDeps,
	session *types.Session,
	meta *types.SessionMeta,
	requestID int,
	decision string,
	responses []string,
	_ map[string]any,
) error {
	if session == nil {
		return invalidError("session is required", nil)
	}
	if deps.liveManager == nil {
		return unavailableError("live manager not available", nil)
	}
	if requestID < 0 {
		return invalidError("request id is required", nil)
	}

	record, ok, err := claudeApprovalRecord(ctx, deps.approvalStore, session.ID, requestID)
	if err != nil {
		return unavailableError(err.Error(), err)
	}
	if !ok {
		return invalidError("approval request not available", nil)
	}

	prompt := claudeApprovalDecisionPrompt(record, decision, responses)
	if strings.TrimSpace(prompt) == "" {
		return invalidError("approval prompt is required", nil)
	}

	turnID, err := deps.liveManager.StartTurn(ctx, session, meta, []map[string]any{
		{"type": "text", "text": prompt},
	}, nil)
	if err != nil {
		return invalidError(err.Error(), err)
	}
	if deps.approvalStore != nil {
		_ = deps.approvalStore.Delete(ctx, session.ID, requestID)
	}
	if deps.sessionMetaStore != nil {
		now := time.Now().UTC()
		_, _ = deps.sessionMetaStore.Upsert(ctx, &types.SessionMeta{
			SessionID:    session.ID,
			LastTurnID:   turnID,
			LastActiveAt: &now,
		})
	}
	return nil
}

func claudeApprovalRecord(ctx context.Context, approvals ApprovalStore, sessionID string, requestID int) (*types.Approval, bool, error) {
	if approvals == nil {
		return nil, false, nil
	}
	return approvals.Get(ctx, sessionID, requestID)
}

func claudeApprovalDecisionPrompt(record *types.Approval, decision string, responses []string) string {
	joinedResponses := strings.TrimSpace(strings.Join(responses, "\n\n"))
	if joinedResponses != "" {
		return joinedResponses
	}

	params := map[string]any{}
	if record != nil && len(record.Params) > 0 {
		_ = json.Unmarshal(record.Params, &params)
	}
	title := strings.TrimSpace(asString(params["title"]))
	if title == "" {
		title = "the plan"
	}

	switch strings.ToLower(strings.TrimSpace(decision)) {
	case "accept", "accepted", "approve", "approved":
		return "The plan is approved. Exit plan mode and start implementing " + title + "."
	case "decline", "declined", "reject", "rejected":
		return "The plan is not approved yet. Stay in planning mode, revise " + title + ", and then present an updated plan."
	default:
		return ""
	}
}

func parseApprovalTime(raw any) time.Time {
	switch value := raw.(type) {
	case string:
		value = strings.TrimSpace(value)
		if value == "" {
			return time.Time{}
		}
		if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}
