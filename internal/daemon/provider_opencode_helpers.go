package daemon

import (
	"encoding/json"
	"hash/fnv"
	"sort"
	"strconv"
	"strings"
	"time"

	"control/internal/types"
)

func mapOpenCodeEventToCodex(raw string, sessionID string, usedPermissionIDs map[int]string) []types.CodexEvent {
	var event struct {
		Type       string         `json:"type"`
		Properties map[string]any `json:"properties"`
	}
	if err := json.Unmarshal([]byte(raw), &event); err != nil {
		return nil
	}
	eventType := strings.TrimSpace(strings.ToLower(event.Type))
	if eventType == "" {
		return nil
	}
	props := event.Properties
	if props == nil {
		props = map[string]any{}
	}
	now := time.Now().UTC()
	build := func(method string, id *int, payload map[string]any) types.CodexEvent {
		var params json.RawMessage
		if len(payload) > 0 {
			params, _ = json.Marshal(payload)
		}
		return types.CodexEvent{
			ID:     id,
			Method: method,
			Params: params,
			TS:     now.Format(time.RFC3339Nano),
		}
	}
	if sid := openCodeEventSessionID(eventType, props); sid != "" && sid != sessionID {
		return nil
	}

	switch eventType {
	case "session.status":
		status, _ := props["status"].(map[string]any)
		statusType := strings.ToLower(strings.TrimSpace(asString(status["type"])))
		switch statusType {
		case "busy":
			return []types.CodexEvent{build("turn/started", nil, map[string]any{
				"turn": map[string]any{"status": "in_progress"},
			})}
		case "idle":
			return []types.CodexEvent{build("turn/completed", nil, map[string]any{
				"turn": map[string]any{"status": "completed"},
			})}
		default:
			return nil
		}
	case "session.idle":
		return []types.CodexEvent{build("turn/completed", nil, map[string]any{
			"turn": map[string]any{"status": "completed"},
		})}
	case "session.error":
		errData := map[string]any{}
		if rawErr, ok := props["error"].(map[string]any); ok && rawErr != nil {
			errData = rawErr
		}
		msg := openCodeEventErrorMessage(errData)
		if msg == "" {
			msg = "session error"
		}
		events := []types.CodexEvent{
			build("error", nil, map[string]any{"error": map[string]any{"message": msg}}),
		}
		name := strings.ToLower(strings.TrimSpace(asString(errData["name"])))
		if name == "messageabortederror" {
			events = append(events, build("turn/completed", nil, map[string]any{
				"turn": map[string]any{"status": "interrupted"},
			}))
		}
		return events
	case "message.part.updated":
		part, _ := props["part"].(map[string]any)
		if part == nil {
			return nil
		}
		partType := strings.ToLower(strings.TrimSpace(asString(part["type"])))
		switch partType {
		case "step-start":
			item := map[string]any{
				"id":   strings.TrimSpace(asString(part["messageID"])),
				"type": "agentMessage",
			}
			return []types.CodexEvent{build("item/started", nil, map[string]any{"item": item})}
		case "step-finish":
			item := map[string]any{
				"id":   strings.TrimSpace(asString(part["messageID"])),
				"type": "agentMessage",
			}
			return []types.CodexEvent{build("item/completed", nil, map[string]any{"item": item})}
		case "text":
			delta := strings.TrimSpace(asString(props["delta"]))
			if delta == "" {
				delta = strings.TrimSpace(asString(part["text"]))
			}
			if delta == "" {
				return nil
			}
			return []types.CodexEvent{build("item/agentMessage/delta", nil, map[string]any{"delta": delta})}
		case "reasoning":
			text := strings.TrimSpace(asString(part["text"]))
			if text == "" {
				return nil
			}
			item := map[string]any{
				"id":   strings.TrimSpace(asString(part["id"])),
				"type": "reasoning",
				"text": text,
			}
			return []types.CodexEvent{build("item/updated", nil, map[string]any{"item": item})}
		default:
			return nil
		}
	case "permission.updated":
		permissionID := strings.TrimSpace(asString(props["id"]))
		if permissionID == "" {
			return nil
		}
		permission := openCodePermission{
			PermissionID: permissionID,
			SessionID:    strings.TrimSpace(asString(props["sessionID"])),
			Kind:         strings.TrimSpace(asString(props["type"])),
			Summary:      strings.TrimSpace(asString(props["title"])),
			CreatedAt:    openCodePermissionCreatedAt(props),
			Raw:          props,
		}
		metadata, _ := props["metadata"].(map[string]any)
		if metadata != nil {
			if permission.Command == "" {
				permission.Command = strings.TrimSpace(asString(metadata["command"]))
			}
			if permission.Command == "" {
				permission.Command = strings.TrimSpace(asString(metadata["parsedCmd"]))
			}
			if permission.Reason == "" {
				permission.Reason = strings.TrimSpace(asString(metadata["reason"]))
			}
		}
		method := openCodePermissionMethod(permission)
		requestID := openCodePermissionRequestID(permission.PermissionID, usedPermissionIDs)
		params := map[string]any{
			"permission_id": permission.PermissionID,
			"session_id":    permission.SessionID,
			"type":          permission.Kind,
			"title":         permission.Summary,
		}
		switch method {
		case "item/commandExecution/requestApproval":
			if permission.Command != "" {
				params["parsedCmd"] = permission.Command
			}
		case "item/fileChange/requestApproval":
			if permission.Reason != "" {
				params["reason"] = permission.Reason
			}
		default:
			if permission.Summary != "" {
				params["questions"] = []map[string]any{
					{"text": permission.Summary},
				}
			}
		}
		return []types.CodexEvent{build(method, &requestID, params)}
	case "permission.replied":
		permissionID := strings.TrimSpace(asString(props["permissionID"]))
		if permissionID == "" {
			return nil
		}
		requestID := openCodePermissionRequestID(permissionID, usedPermissionIDs)
		return []types.CodexEvent{build("permission/replied", &requestID, map[string]any{
			"permission_id": permissionID,
			"request_id":    requestID,
			"response":      strings.TrimSpace(asString(props["response"])),
		})}
	default:
		return nil
	}
}

func openCodeEventSessionID(eventType string, properties map[string]any) string {
	if properties == nil {
		return ""
	}
	switch eventType {
	case "session.status", "session.idle", "session.compacted", "session.error":
		return strings.TrimSpace(asString(properties["sessionID"]))
	case "message.updated":
		info, _ := properties["info"].(map[string]any)
		return strings.TrimSpace(asString(info["sessionID"]))
	case "message.part.updated", "message.part.removed":
		part, _ := properties["part"].(map[string]any)
		return strings.TrimSpace(asString(part["sessionID"]))
	case "permission.updated", "permission.replied":
		return strings.TrimSpace(asString(properties["sessionID"]))
	default:
		return ""
	}
}

func openCodeEventErrorMessage(raw map[string]any) string {
	if raw == nil {
		return ""
	}
	if msg := strings.TrimSpace(asString(raw["message"])); msg != "" {
		return msg
	}
	data, _ := raw["data"].(map[string]any)
	if data != nil {
		if msg := strings.TrimSpace(asString(data["message"])); msg != "" {
			return msg
		}
	}
	return ""
}

func openCodePermissionCreatedAt(raw map[string]any) time.Time {
	if raw == nil {
		return time.Time{}
	}
	if when := openCodeTimestamp(raw["createdAt"]); !when.IsZero() {
		return when
	}
	if when := openCodeTimestamp(raw["ts"]); !when.IsZero() {
		return when
	}
	if clock, ok := raw["time"].(map[string]any); ok && clock != nil {
		if when := openCodeTimestamp(clock["created"]); !when.IsZero() {
			return when
		}
	}
	return time.Time{}
}

func openCodeModelID(providerID string, entry any) string {
	switch value := entry.(type) {
	case string:
		return openCodeNormalizedModelID(providerID, value)
	case map[string]any:
		modelID := strings.TrimSpace(asString(value["id"]))
		if modelID == "" {
			modelID = strings.TrimSpace(asString(value["modelID"]))
		}
		return openCodeNormalizedModelID(providerID, modelID)
	default:
		return ""
	}
}

func openCodeRawModelID(entry any) string {
	switch value := entry.(type) {
	case string:
		return strings.TrimSpace(value)
	case map[string]any:
		modelID := strings.TrimSpace(asString(value["id"]))
		if modelID == "" {
			modelID = strings.TrimSpace(asString(value["modelID"]))
		}
		return modelID
	default:
		return ""
	}
}

func openCodeModelEntries(raw any) []any {
	switch typed := raw.(type) {
	case []any:
		return typed
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		out := make([]any, 0, len(keys))
		for _, key := range keys {
			entry := typed[key]
			if mapped, ok := entry.(map[string]any); ok {
				modelID := strings.TrimSpace(asString(mapped["id"]))
				if modelID == "" {
					modelID = strings.TrimSpace(asString(mapped["modelID"]))
				}
				if modelID == "" {
					cloned := make(map[string]any, len(mapped)+1)
					for k, v := range mapped {
						cloned[k] = v
					}
					cloned["id"] = key
					out = append(out, cloned)
					continue
				}
				out = append(out, mapped)
				continue
			}
			if modelID := strings.TrimSpace(asString(entry)); modelID != "" {
				out = append(out, modelID)
				continue
			}
			out = append(out, map[string]any{"id": key})
		}
		return out
	default:
		return nil
	}
}

func openCodeNormalizedModelID(providerID, modelID string) string {
	providerID = strings.TrimSpace(providerID)
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return ""
	}
	if providerID == "" {
		return modelID
	}
	if strings.HasPrefix(modelID, providerID+"/") {
		return modelID
	}
	return providerID + "/" + modelID
}

func openCodeParseProviderCatalog(providers []map[string]any, defaults map[string]any) openCodeParsedProviderCatalog {
	out := openCodeParsedProviderCatalog{
		ProviderIDs:      map[string]struct{}{},
		ModelToProvider:  map[string]string{},
		NormalizedModels: []string{},
	}
	seen := map[string]struct{}{}
	for _, provider := range providers {
		if provider == nil {
			continue
		}
		providerID := strings.TrimSpace(asString(provider["id"]))
		if providerID == "" {
			providerID = strings.TrimSpace(asString(provider["providerID"]))
		}
		if providerID == "" {
			continue
		}
		out.ProviderIDs[providerID] = struct{}{}
		for _, entry := range openCodeModelEntries(provider["models"]) {
			rawModelID := openCodeRawModelID(entry)
			if rawModelID != "" {
				if _, exists := out.ModelToProvider[rawModelID]; !exists {
					out.ModelToProvider[rawModelID] = providerID
				}
			}
			modelID := openCodeModelID(providerID, entry)
			if modelID == "" {
				continue
			}
			if _, exists := seen[modelID]; exists {
				continue
			}
			seen[modelID] = struct{}{}
			out.NormalizedModels = append(out.NormalizedModels, modelID)
		}
		if out.DefaultModel == "" {
			if value, ok := defaults[providerID]; ok {
				out.DefaultModel = openCodeNormalizedModelID(providerID, strings.TrimSpace(asString(value)))
			}
		}
	}
	if out.DefaultModel != "" {
		sort.SliceStable(out.NormalizedModels, func(i, j int) bool {
			left := out.NormalizedModels[i]
			right := out.NormalizedModels[j]
			if left == out.DefaultModel {
				return true
			}
			if right == out.DefaultModel {
				return false
			}
			return i < j
		})
	}
	return out
}

func extractOpenCodePartsText(parts []map[string]any) string {
	if len(parts) == 0 {
		return ""
	}
	texts := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == nil {
			continue
		}
		typ := strings.ToLower(strings.TrimSpace(asString(part["type"])))
		if typ != "" && typ != "text" {
			continue
		}
		text := strings.TrimSpace(asString(part["text"]))
		if text == "" {
			continue
		}
		texts = append(texts, text)
	}
	return strings.TrimSpace(strings.Join(texts, "\n"))
}

func extractOpenCodeSessionMessageText(message openCodeSessionMessage) string {
	if text := extractOpenCodePartsText(message.Parts); text != "" {
		return text
	}
	if message.Message != nil {
		if text := strings.TrimSpace(extractClaudeMessageText(message.Message)); text != "" {
			return text
		}
	}
	return ""
}

func normalizeOpenCodeSessionMessages(payload any) []openCodeSessionMessage {
	switch typed := payload.(type) {
	case []any:
		return toOpenCodeSessionMessageSlice(typed)
	case map[string]any:
		for _, key := range []string{"messages", "items", "data"} {
			if list, ok := typed[key].([]any); ok {
				return toOpenCodeSessionMessageSlice(list)
			}
		}
		if parsed, ok := parseOpenCodeSessionMessage(typed); ok {
			return []openCodeSessionMessage{parsed}
		}
	default:
		return nil
	}
	return nil
}

func toOpenCodeSessionMessageSlice(values []any) []openCodeSessionMessage {
	out := make([]openCodeSessionMessage, 0, len(values))
	for _, value := range values {
		entry, ok := value.(map[string]any)
		if !ok || entry == nil {
			continue
		}
		parsed, ok := parseOpenCodeSessionMessage(entry)
		if !ok {
			continue
		}
		out = append(out, parsed)
	}
	return out
}

func parseOpenCodeSessionMessage(raw map[string]any) (openCodeSessionMessage, bool) {
	if raw == nil {
		return openCodeSessionMessage{}, false
	}
	info, _ := raw["info"].(map[string]any)
	message, _ := raw["message"].(map[string]any)
	if info == nil {
		info = map[string]any{}
		if role := strings.TrimSpace(asString(raw["role"])); role != "" {
			info["role"] = role
		}
		if id := strings.TrimSpace(asString(raw["id"])); id != "" {
			info["id"] = id
		}
		if created := raw["createdAt"]; created != nil {
			info["createdAt"] = created
		}
	}
	parts := toOpenCodeMapSlice(openCodeModelEntries(raw["parts"]))
	if len(parts) == 0 && message != nil {
		parts = toOpenCodeMapSlice(openCodeModelEntries(message["content"]))
	}
	if len(parts) == 0 && len(info) == 0 && message == nil {
		return openCodeSessionMessage{}, false
	}
	return openCodeSessionMessage{
		Info:    info,
		Parts:   parts,
		Message: message,
	}, true
}

func openCodeLatestAssistantSnapshot(messages []openCodeSessionMessage) openCodeAssistantSnapshot {
	var (
		best      openCodeAssistantSnapshot
		bestSet   bool
		bestIndex = -1
	)
	for i, message := range messages {
		role := strings.ToLower(strings.TrimSpace(openCodeSessionMessageRole(message)))
		if role != "assistant" && role != "model" {
			continue
		}
		text := extractOpenCodeSessionMessageText(message)
		if text == "" {
			continue
		}
		candidate := openCodeAssistantSnapshot{
			MessageID: openCodeSessionMessageID(message),
			Text:      text,
			CreatedAt: openCodeSessionMessageCreatedAt(message),
		}
		if !bestSet {
			best = candidate
			bestSet = true
			bestIndex = i
			continue
		}
		if candidate.CreatedAt.After(best.CreatedAt) {
			best = candidate
			bestIndex = i
			continue
		}
		if candidate.CreatedAt.Equal(best.CreatedAt) && i > bestIndex {
			best = candidate
			bestIndex = i
		}
	}
	return best
}

func openCodeSessionMessageRole(message openCodeSessionMessage) string {
	if message.Info != nil {
		if role := strings.TrimSpace(asString(message.Info["role"])); role != "" {
			return role
		}
		if role := strings.TrimSpace(asString(message.Info["type"])); role != "" {
			return role
		}
	}
	if message.Message != nil {
		if role := strings.TrimSpace(asString(message.Message["role"])); role != "" {
			return role
		}
	}
	return ""
}

func openCodeSessionMessageID(message openCodeSessionMessage) string {
	if message.Info != nil {
		if id := strings.TrimSpace(asString(message.Info["id"])); id != "" {
			return id
		}
		if id := strings.TrimSpace(asString(message.Info["messageID"])); id != "" {
			return id
		}
	}
	if message.Message != nil {
		if id := strings.TrimSpace(asString(message.Message["id"])); id != "" {
			return id
		}
	}
	return ""
}

func openCodeSessionMessageCreatedAt(message openCodeSessionMessage) time.Time {
	if message.Info != nil {
		if when := openCodeTimestamp(message.Info["createdAt"]); !when.IsZero() {
			return when
		}
		if when := openCodeTimestamp(message.Info["ts"]); !when.IsZero() {
			return when
		}
	}
	if message.Message != nil {
		if when := openCodeTimestamp(message.Message["createdAt"]); !when.IsZero() {
			return when
		}
	}
	return time.Time{}
}

func openCodeSessionMessagesToItems(messages []openCodeSessionMessage) []map[string]any {
	if len(messages) == 0 {
		return nil
	}
	items := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		text := strings.TrimSpace(extractOpenCodeSessionMessageText(message))
		if text == "" {
			continue
		}
		role := strings.ToLower(strings.TrimSpace(openCodeSessionMessageRole(message)))
		item := map[string]any{}
		switch role {
		case "user":
			item["type"] = "userMessage"
			item["content"] = []map[string]any{
				{"type": "text", "text": text},
			}
		case "assistant", "model":
			item["type"] = "assistant"
			item["message"] = map[string]any{
				"content": []map[string]any{
					{"type": "text", "text": text},
				},
			}
		default:
			continue
		}
		if messageID := strings.TrimSpace(openCodeSessionMessageID(message)); messageID != "" {
			item["provider_message_id"] = messageID
		}
		if createdAt := openCodeSessionMessageCreatedAt(message); !createdAt.IsZero() {
			item["provider_created_at"] = createdAt.UTC().Format(time.RFC3339Nano)
		}
		items = append(items, item)
	}
	return items
}

func trimItemsToLimit(items []map[string]any, limit int) []map[string]any {
	if limit <= 0 || len(items) <= limit {
		return items
	}
	return items[len(items)-limit:]
}

func openCodeMissingHistoryItems(localItems, remoteItems []map[string]any) []map[string]any {
	if len(remoteItems) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	for _, item := range localItems {
		if key := openCodeHistoryItemKey(item); key != "" {
			seen[key] = struct{}{}
		}
	}
	missing := make([]map[string]any, 0, len(remoteItems))
	for _, item := range remoteItems {
		key := openCodeHistoryItemKey(item)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		missing = append(missing, item)
	}
	return missing
}

func openCodeHistoryItemKey(item map[string]any) string {
	if item == nil {
		return ""
	}
	if messageID := strings.TrimSpace(asString(item["provider_message_id"])); messageID != "" {
		return "id:" + messageID
	}
	itemType := strings.ToLower(strings.TrimSpace(asString(item["type"])))
	if itemType == "" {
		return ""
	}
	text := openCodeHistoryItemText(item)
	if text == "" {
		return ""
	}
	if createdAt := strings.TrimSpace(asString(item["provider_created_at"])); createdAt != "" {
		return "type:" + itemType + "|created_at:" + createdAt + "|text:" + text
	}
	return "type:" + itemType + "|text:" + text
}

func openCodeHistoryItemText(item map[string]any) string {
	if item == nil {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(asString(item["type"]))) {
	case "usermessage":
		return strings.TrimSpace(openCodeContentText(item["content"]))
	case "assistant":
		if message, ok := item["message"].(map[string]any); ok {
			if text := strings.TrimSpace(extractClaudeMessageText(message)); text != "" {
				return text
			}
			return strings.TrimSpace(openCodeContentText(message["content"]))
		}
	}
	if text := strings.TrimSpace(asString(item["text"])); text != "" {
		return text
	}
	return strings.TrimSpace(openCodeContentText(item["content"]))
}

func openCodeContentText(raw any) string {
	switch typed := raw.(type) {
	case string:
		return strings.TrimSpace(typed)
	case []any:
		parts := make([]string, 0, len(typed))
		for _, entry := range typed {
			block, ok := entry.(map[string]any)
			if !ok || block == nil {
				continue
			}
			if text := strings.TrimSpace(asString(block["text"])); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.TrimSpace(strings.Join(parts, ""))
	case []map[string]any:
		parts := make([]string, 0, len(typed))
		for _, block := range typed {
			if block == nil {
				continue
			}
			if text := strings.TrimSpace(asString(block["text"])); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.TrimSpace(strings.Join(parts, ""))
	default:
		return ""
	}
}

func openCodeAssistantChanged(current, baseline openCodeAssistantSnapshot) bool {
	if strings.TrimSpace(current.Text) == "" {
		return false
	}
	currentID := strings.TrimSpace(current.MessageID)
	baselineID := strings.TrimSpace(baseline.MessageID)
	if currentID != "" && baselineID != "" {
		return currentID != baselineID
	}
	if currentID != "" && baselineID == "" {
		return true
	}
	if baseline.Text == "" {
		return true
	}
	return current.Text != baseline.Text
}

func openCodeSessionCreatedAt(raw map[string]any) time.Time {
	if raw == nil {
		return time.Time{}
	}
	if when := openCodeTimestamp(raw["createdAt"]); !when.IsZero() {
		return when
	}
	if when := openCodeTimestamp(raw["ts"]); !when.IsZero() {
		return when
	}
	if clock, ok := raw["time"].(map[string]any); ok && clock != nil {
		if when := openCodeTimestamp(clock["created"]); !when.IsZero() {
			return when
		}
	}
	return time.Time{}
}

func normalizeOpenCodePermissionList(payload any) []map[string]any {
	switch typed := payload.(type) {
	case []any:
		return toOpenCodeMapSlice(typed)
	case map[string]any:
		for _, key := range []string{"permissions", "data", "items"} {
			if list, ok := typed[key].([]any); ok {
				return toOpenCodeMapSlice(list)
			}
		}
		return []map[string]any{typed}
	default:
		return nil
	}
}

func toOpenCodeMapSlice(values []any) []map[string]any {
	out := make([]map[string]any, 0, len(values))
	for _, value := range values {
		entry, ok := value.(map[string]any)
		if !ok || entry == nil {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func parseOpenCodePermission(item map[string]any) (openCodePermission, bool) {
	if item == nil {
		return openCodePermission{}, false
	}
	permissionID := strings.TrimSpace(asString(item["id"]))
	if permissionID == "" {
		permissionID = strings.TrimSpace(asString(item["permissionID"]))
	}
	if permissionID == "" {
		return openCodePermission{}, false
	}

	sessionID := strings.TrimSpace(asString(item["sessionID"]))
	if sessionID == "" {
		sessionID = strings.TrimSpace(asString(item["sessionId"]))
	}
	if sessionID == "" {
		if session, ok := item["session"].(map[string]any); ok {
			sessionID = strings.TrimSpace(asString(session["id"]))
		}
	}

	status := strings.ToLower(strings.TrimSpace(asString(item["status"])))
	kind := strings.TrimSpace(asString(item["type"]))
	if kind == "" {
		kind = strings.TrimSpace(asString(item["kind"]))
	}
	summary := strings.TrimSpace(asString(item["message"]))
	command := strings.TrimSpace(asString(item["command"]))
	if command == "" {
		command = strings.TrimSpace(asString(item["parsedCmd"]))
	}
	reason := strings.TrimSpace(asString(item["reason"]))
	createdAt := openCodeTimestamp(item["createdAt"])
	if createdAt.IsZero() {
		createdAt = openCodeTimestamp(item["ts"])
	}
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	return openCodePermission{
		PermissionID: permissionID,
		SessionID:    sessionID,
		Status:       status,
		Kind:         kind,
		Summary:      summary,
		Command:      command,
		Reason:       reason,
		CreatedAt:    createdAt,
		Raw:          item,
	}, true
}

func openCodeTimestamp(raw any) time.Time {
	parseUnix := func(value int64) time.Time {
		switch {
		case value >= 1_000_000_000_000_000_000:
			return time.Unix(0, value).UTC()
		case value >= 1_000_000_000_000_000:
			return time.UnixMicro(value).UTC()
		case value >= 1_000_000_000_000:
			return time.UnixMilli(value).UTC()
		default:
			return time.Unix(value, 0).UTC()
		}
	}
	switch value := raw.(type) {
	case string:
		if parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(value)); err == nil {
			return parsed.UTC()
		}
		if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value)); err == nil {
			return parsed.UTC()
		}
	case float64:
		return parseUnix(int64(value))
	case int64:
		return parseUnix(value)
	case int:
		return parseUnix(int64(value))
	case json.Number:
		if i, err := strconv.ParseInt(value.String(), 10, 64); err == nil {
			return parseUnix(i)
		}
	}
	return time.Time{}
}

func openCodePermissionMethod(permission openCodePermission) string {
	kind := strings.ToLower(strings.TrimSpace(permission.Kind))
	switch {
	case strings.Contains(kind, "command"), strings.Contains(kind, "exec"), strings.Contains(kind, "shell"):
		return "item/commandExecution/requestApproval"
	case strings.Contains(kind, "file"), strings.Contains(kind, "write"), strings.Contains(kind, "edit"):
		return "item/fileChange/requestApproval"
	default:
		return "tool/requestUserInput"
	}
}

func openCodePermissionRequestID(permissionID string, used map[int]string) int {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(strings.TrimSpace(permissionID)))
	base := int(hash.Sum32() & 0x7fffffff)
	if base == 0 {
		base = 1
	}
	candidate := base
	for {
		if existing, ok := used[candidate]; !ok || existing == permissionID {
			used[candidate] = permissionID
			return candidate
		}
		candidate++
		if candidate <= 0 {
			candidate = 1
		}
	}
}

func normalizeApprovalDecision(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "accept", "approved", "allow", "yes":
		return "accept"
	case "decline", "deny", "rejected", "no":
		return "decline"
	default:
		return value
	}
}

func normalizeOpenCodePermissionResponse(raw string) string {
	switch normalizeApprovalDecision(raw) {
	case "accept":
		return "once"
	case "decline":
		return "reject"
	default:
		return "once"
	}
}
