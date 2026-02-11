package app

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"control/internal/types"
)

type ApprovalRequest struct {
	RequestID int
	SessionID string
	Method    string
	Summary   string
	Detail    string
	CreatedAt time.Time
}

type ApprovalResolution struct {
	RequestID  int
	SessionID  string
	Method     string
	Summary    string
	Detail     string
	Decision   string
	ResolvedAt time.Time
}

func approvalSummary(method string, params map[string]any) (string, string) {
	switch method {
	case "item/commandExecution/requestApproval":
		cmd := asString(params["parsedCmd"])
		if cmd == "" {
			cmd = asString(params["command"])
		}
		cmd = strings.TrimSpace(cmd)
		if cmd == "" {
			return "command execution", ""
		}
		return "command", cmd
	case "item/fileChange/requestApproval":
		reason := strings.TrimSpace(asString(params["reason"]))
		if reason != "" {
			return "file change", reason
		}
		return "file change", ""
	case "tool/requestUserInput":
		if questions, ok := params["questions"].([]any); ok {
			for _, q := range questions {
				if qMap, ok := q.(map[string]any); ok {
					text := strings.TrimSpace(asString(qMap["text"]))
					if text != "" {
						return "user input", text
					}
				}
			}
		}
		return "user input", ""
	default:
	}
	return "approval", ""
}

func approvalFromRecord(record *types.Approval) *ApprovalRequest {
	if record == nil {
		return nil
	}
	params := map[string]any{}
	if len(record.Params) > 0 {
		_ = json.Unmarshal(record.Params, &params)
	}
	summary, detail := approvalSummary(record.Method, params)
	return &ApprovalRequest{
		RequestID: record.RequestID,
		SessionID: record.SessionID,
		Method:    record.Method,
		Summary:   summary,
		Detail:    detail,
		CreatedAt: record.CreatedAt,
	}
}

func cloneApprovalRequest(req *ApprovalRequest) *ApprovalRequest {
	if req == nil {
		return nil
	}
	copy := *req
	return &copy
}

func approvalResolutionFromRequest(req *ApprovalRequest, decision string, resolvedAt time.Time) *ApprovalResolution {
	if req == nil || req.RequestID < 0 {
		return nil
	}
	if resolvedAt.IsZero() {
		resolvedAt = time.Now().UTC()
	}
	return &ApprovalResolution{
		RequestID:  req.RequestID,
		SessionID:  req.SessionID,
		Method:     req.Method,
		Summary:    req.Summary,
		Detail:     req.Detail,
		Decision:   strings.TrimSpace(strings.ToLower(decision)),
		ResolvedAt: resolvedAt,
	}
}

func approvalRequestsFromRecords(records []*types.Approval) []*ApprovalRequest {
	if len(records) == 0 {
		return nil
	}
	requests := make([]*ApprovalRequest, 0, len(records))
	for _, record := range records {
		req := approvalFromRecord(record)
		if req == nil || req.RequestID < 0 {
			continue
		}
		requests = append(requests, req)
	}
	return normalizeApprovalRequests(requests)
}

func normalizeApprovalRequests(requests []*ApprovalRequest) []*ApprovalRequest {
	if len(requests) == 0 {
		return nil
	}
	byRequestID := map[int]*ApprovalRequest{}
	for _, req := range requests {
		if req == nil || req.RequestID < 0 {
			continue
		}
		existing := byRequestID[req.RequestID]
		if existing == nil || req.CreatedAt.After(existing.CreatedAt) {
			byRequestID[req.RequestID] = cloneApprovalRequest(req)
		}
	}
	if len(byRequestID) == 0 {
		return nil
	}
	normalized := make([]*ApprovalRequest, 0, len(byRequestID))
	for _, req := range byRequestID {
		normalized = append(normalized, req)
	}
	sort.Slice(normalized, func(i, j int) bool {
		left := normalized[i]
		right := normalized[j]
		if left.CreatedAt.Equal(right.CreatedAt) {
			return left.RequestID < right.RequestID
		}
		return left.CreatedAt.Before(right.CreatedAt)
	})
	return normalized
}

func latestApprovalRequest(requests []*ApprovalRequest) *ApprovalRequest {
	if len(requests) == 0 {
		return nil
	}
	return cloneApprovalRequest(requests[len(requests)-1])
}

func cloneApprovalResolution(resolution *ApprovalResolution) *ApprovalResolution {
	if resolution == nil {
		return nil
	}
	copy := *resolution
	return &copy
}

func upsertApprovalRequest(requests []*ApprovalRequest, req *ApprovalRequest) ([]*ApprovalRequest, bool) {
	if req == nil || req.RequestID < 0 {
		return requests, false
	}
	next := make([]*ApprovalRequest, 0, len(requests)+1)
	found := false
	changed := false
	for _, existing := range requests {
		if existing == nil || existing.RequestID < 0 {
			continue
		}
		if existing.RequestID != req.RequestID {
			next = append(next, cloneApprovalRequest(existing))
			continue
		}
		found = true
		if approvalRequestEqual(existing, req) {
			next = append(next, cloneApprovalRequest(existing))
			continue
		}
		next = append(next, cloneApprovalRequest(req))
		changed = true
	}
	if !found {
		next = append(next, cloneApprovalRequest(req))
		changed = true
	}
	next = normalizeApprovalRequests(next)
	return next, changed
}

func removeApprovalRequest(requests []*ApprovalRequest, requestID int) ([]*ApprovalRequest, bool) {
	if len(requests) == 0 {
		return nil, false
	}
	next := make([]*ApprovalRequest, 0, len(requests))
	removed := false
	for _, req := range requests {
		if req == nil {
			continue
		}
		if req.RequestID == requestID {
			removed = true
			continue
		}
		next = append(next, cloneApprovalRequest(req))
	}
	next = normalizeApprovalRequests(next)
	return next, removed
}

func findApprovalRequestByID(requests []*ApprovalRequest, requestID int) *ApprovalRequest {
	for _, req := range requests {
		if req == nil || req.RequestID != requestID {
			continue
		}
		return cloneApprovalRequest(req)
	}
	return nil
}

func normalizeApprovalResolutions(resolutions []*ApprovalResolution) []*ApprovalResolution {
	if len(resolutions) == 0 {
		return nil
	}
	byRequestID := map[int]*ApprovalResolution{}
	for _, resolution := range resolutions {
		if resolution == nil || resolution.RequestID < 0 {
			continue
		}
		existing := byRequestID[resolution.RequestID]
		if existing == nil || resolution.ResolvedAt.After(existing.ResolvedAt) {
			byRequestID[resolution.RequestID] = cloneApprovalResolution(resolution)
		}
	}
	if len(byRequestID) == 0 {
		return nil
	}
	normalized := make([]*ApprovalResolution, 0, len(byRequestID))
	for _, resolution := range byRequestID {
		normalized = append(normalized, resolution)
	}
	sort.Slice(normalized, func(i, j int) bool {
		left := normalized[i]
		right := normalized[j]
		if left.ResolvedAt.Equal(right.ResolvedAt) {
			return left.RequestID < right.RequestID
		}
		return left.ResolvedAt.Before(right.ResolvedAt)
	})
	return normalized
}

func upsertApprovalResolution(resolutions []*ApprovalResolution, resolution *ApprovalResolution) ([]*ApprovalResolution, bool) {
	if resolution == nil || resolution.RequestID < 0 {
		return resolutions, false
	}
	next := make([]*ApprovalResolution, 0, len(resolutions)+1)
	found := false
	changed := false
	for _, existing := range resolutions {
		if existing == nil || existing.RequestID < 0 {
			continue
		}
		if existing.RequestID != resolution.RequestID {
			next = append(next, cloneApprovalResolution(existing))
			continue
		}
		found = true
		if approvalResolutionEqual(existing, resolution) {
			next = append(next, cloneApprovalResolution(existing))
			continue
		}
		next = append(next, cloneApprovalResolution(resolution))
		changed = true
	}
	if !found {
		next = append(next, cloneApprovalResolution(resolution))
		changed = true
	}
	next = normalizeApprovalResolutions(next)
	return next, changed
}

func removeApprovalResolution(resolutions []*ApprovalResolution, requestID int) ([]*ApprovalResolution, bool) {
	if len(resolutions) == 0 {
		return nil, false
	}
	next := make([]*ApprovalResolution, 0, len(resolutions))
	removed := false
	for _, resolution := range resolutions {
		if resolution == nil {
			continue
		}
		if resolution.RequestID == requestID {
			removed = true
			continue
		}
		next = append(next, cloneApprovalResolution(resolution))
	}
	next = normalizeApprovalResolutions(next)
	return next, removed
}

func approvalRequestEqual(left, right *ApprovalRequest) bool {
	if left == nil || right == nil {
		return left == right
	}
	return left.RequestID == right.RequestID &&
		left.SessionID == right.SessionID &&
		left.Method == right.Method &&
		left.Summary == right.Summary &&
		left.Detail == right.Detail &&
		left.CreatedAt.Equal(right.CreatedAt)
}

func approvalResolutionEqual(left, right *ApprovalResolution) bool {
	if left == nil || right == nil {
		return left == right
	}
	return left.RequestID == right.RequestID &&
		left.SessionID == right.SessionID &&
		left.Method == right.Method &&
		left.Summary == right.Summary &&
		left.Detail == right.Detail &&
		left.Decision == right.Decision &&
		left.ResolvedAt.Equal(right.ResolvedAt)
}

func mergeApprovalBlocks(blocks []ChatBlock, requests []*ApprovalRequest, resolutions []*ApprovalResolution) []ChatBlock {
	requests = normalizeApprovalRequests(requests)
	resolutions = normalizeApprovalResolutions(resolutions)
	requestByID := map[int]*ApprovalRequest{}
	for _, req := range requests {
		if req == nil || req.RequestID < 0 {
			continue
		}
		requestByID[req.RequestID] = req
	}
	resolutionByID := map[int]*ApprovalResolution{}
	for _, resolution := range resolutions {
		if resolution == nil || resolution.RequestID < 0 {
			continue
		}
		resolutionByID[resolution.RequestID] = resolution
	}

	out := make([]ChatBlock, 0, len(blocks)+len(requests)+len(resolutions))
	consumedRequests := map[int]struct{}{}
	consumedResolutions := map[int]struct{}{}
	for _, block := range blocks {
		if block.Role != ChatRoleApproval && block.Role != ChatRoleApprovalResolved {
			out = append(out, block)
			continue
		}
		requestID, ok := approvalRequestIDFromBlock(block)
		if !ok {
			continue
		}
		if resolution, exists := resolutionByID[requestID]; exists {
			out = append(out, approvalResolutionToBlock(resolution))
			consumedResolutions[requestID] = struct{}{}
			continue
		}
		if req, exists := requestByID[requestID]; exists {
			out = append(out, approvalRequestToBlock(req))
			consumedRequests[requestID] = struct{}{}
			continue
		}
		out = append(out, block)
	}
	for _, resolution := range resolutions {
		if resolution == nil || resolution.RequestID < 0 {
			continue
		}
		if _, seen := consumedResolutions[resolution.RequestID]; seen {
			continue
		}
		out = append(out, approvalResolutionToBlock(resolution))
	}
	for _, req := range requests {
		if req == nil || req.RequestID < 0 {
			continue
		}
		if _, seen := consumedRequests[req.RequestID]; seen {
			continue
		}
		out = append(out, approvalRequestToBlock(req))
	}
	return out
}

func preserveApprovalPositions(previous []ChatBlock, next []ChatBlock) []ChatBlock {
	if len(previous) == 0 || len(next) == 0 {
		return next
	}
	anchorByID := map[int]int{}
	nonApprovalCount := 0
	for _, block := range previous {
		if block.Role != ChatRoleApproval && block.Role != ChatRoleApprovalResolved {
			nonApprovalCount++
			continue
		}
		requestID, ok := approvalRequestIDFromBlock(block)
		if !ok {
			continue
		}
		if _, exists := anchorByID[requestID]; !exists {
			anchorByID[requestID] = nonApprovalCount
		}
	}
	if len(anchorByID) == 0 {
		return next
	}

	nonApproval := make([]ChatBlock, 0, len(next))
	anchored := map[int][]ChatBlock{}
	unanchored := make([]ChatBlock, 0, len(next))
	for _, block := range next {
		if block.Role != ChatRoleApproval && block.Role != ChatRoleApprovalResolved {
			nonApproval = append(nonApproval, block)
			continue
		}
		requestID, ok := approvalRequestIDFromBlock(block)
		if !ok {
			unanchored = append(unanchored, block)
			continue
		}
		anchor, exists := anchorByID[requestID]
		if !exists {
			unanchored = append(unanchored, block)
			continue
		}
		if anchor < 0 {
			anchor = 0
		}
		if anchor > len(nonApproval) {
			anchor = len(nonApproval)
		}
		anchored[anchor] = append(anchored[anchor], block)
	}

	out := make([]ChatBlock, 0, len(next))
	for pos := 0; pos <= len(nonApproval); pos++ {
		if blocks := anchored[pos]; len(blocks) > 0 {
			out = append(out, blocks...)
		}
		if pos < len(nonApproval) {
			out = append(out, nonApproval[pos])
		}
	}
	if len(unanchored) > 0 {
		out = append(out, unanchored...)
	}
	return out
}

func approvalRequestToBlock(req *ApprovalRequest) ChatBlock {
	summary := strings.TrimSpace(req.Summary)
	title := "Approval required"
	if summary != "" {
		title = "Approval required: " + summary
	}
	lines := []string{title}
	if detail := strings.TrimSpace(req.Detail); detail != "" {
		lines = append(lines, "", detail)
	}
	return ChatBlock{
		ID:        approvalBlockID(req.RequestID),
		Role:      ChatRoleApproval,
		Text:      strings.Join(lines, "\n"),
		Status:    ChatStatusNone,
		RequestID: req.RequestID,
		SessionID: req.SessionID,
	}
}

func approvalBlockID(requestID int) string {
	return fmt.Sprintf("approval:%d", requestID)
}

func approvalResolutionToBlock(resolution *ApprovalResolution) ChatBlock {
	status := "resolved"
	switch strings.TrimSpace(strings.ToLower(resolution.Decision)) {
	case "accept", "accepted", "approve", "approved":
		status = "approved"
	case "decline", "declined", "reject", "rejected":
		status = "declined"
	}
	summary := strings.TrimSpace(resolution.Summary)
	title := "Approval " + status
	if summary != "" {
		title = "Approval " + status + ": " + summary
	}
	lines := []string{title}
	if detail := strings.TrimSpace(resolution.Detail); detail != "" {
		lines = append(lines, "", detail)
	}
	return ChatBlock{
		ID:        approvalResolutionBlockID(resolution.RequestID),
		Role:      ChatRoleApprovalResolved,
		Text:      strings.Join(lines, "\n"),
		Status:    ChatStatusNone,
		RequestID: resolution.RequestID,
		SessionID: resolution.SessionID,
	}
}

func approvalResolutionBlockID(requestID int) string {
	return fmt.Sprintf("approval:resolved:%d", requestID)
}

func approvalRequestIDFromBlock(block ChatBlock) (int, bool) {
	if (block.Role == ChatRoleApproval || block.Role == ChatRoleApprovalResolved) && block.RequestID >= 0 {
		return block.RequestID, true
	}
	raw := strings.TrimSpace(block.ID)
	if strings.HasPrefix(raw, "approval:resolved:") {
		raw = strings.TrimPrefix(raw, "approval:resolved:")
		id, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || id < 0 {
			return 0, false
		}
		return id, true
	}
	if !strings.HasPrefix(raw, "approval:") {
		return 0, false
	}
	val := strings.TrimPrefix(raw, "approval:")
	id, err := strconv.Atoi(strings.TrimSpace(val))
	if err != nil || id < 0 {
		return 0, false
	}
	return id, true
}

func approvalSessionIDFromBlock(block ChatBlock) string {
	return strings.TrimSpace(block.SessionID)
}
