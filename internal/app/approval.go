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
	Context   []string
	CreatedAt time.Time
}

type ApprovalResolution struct {
	RequestID  int
	SessionID  string
	Method     string
	Summary    string
	Detail     string
	Context    []string
	Decision   string
	ResolvedAt time.Time
}

type approvalPresentation struct {
	Summary string
	Detail  string
	Context []string
}

func approvalSummary(method string, params map[string]any) (string, string) {
	presentation := approvalPresentationFromParams(method, params)
	return presentation.Summary, presentation.Detail
}

func approvalPresentationFromParams(method string, params map[string]any) approvalPresentation {
	raw := approvalAsMap(params["raw"])
	metadata := approvalMergedMap(approvalAsMap(params["metadata"]), approvalAsMap(raw["metadata"]))
	switch method {
	case "item/commandExecution/requestApproval":
		cmd := approvalFirstNonEmptyString(params, metadata, "parsedCmd", "command", "cmd")
		if cmd == "" {
			p := approvalPresentation{
				Summary: "command execution",
				Context: approvalSharedContextLines(params, metadata),
			}
			if reason := approvalFirstNonEmptyString(params, metadata, "reason", "message", "title", "description"); reason != "" {
				approvalAppendContextLine(&p.Context, "Reason: "+reason)
			}
			return p
		}
		p := approvalPresentation{
			Summary: "command",
			Detail:  cmd,
			Context: approvalSharedContextLines(params, metadata),
		}
		if reason := approvalFirstNonEmptyString(params, metadata, "reason", "message", "title", "description"); reason != "" && !strings.EqualFold(reason, cmd) {
			approvalAppendContextLine(&p.Context, "Reason: "+reason)
		}
		return p
	case "item/fileChange/requestApproval":
		reason := approvalFirstNonEmptyString(params, metadata, "reason", "message", "title", "description")
		p := approvalPresentation{
			Summary: "file change",
			Detail:  reason,
			Context: approvalSharedContextLines(params, metadata),
		}
		for _, path := range approvalExtractPaths(params, metadata) {
			approvalAppendContextLine(&p.Context, "Path: "+path)
		}
		return p
	case "tool/requestUserInput":
		if permissionPresentation, ok := approvalPermissionPresentation(params, raw, metadata); ok {
			return permissionPresentation
		}
		p := approvalPresentation{
			Summary: "user input",
			Context: approvalSharedContextLines(params, metadata),
		}
		questions := approvalExtractQuestions(params, metadata)
		if len(questions) == 0 {
			p.Detail = approvalFirstNonEmptyString(params, metadata, "message", "title", "question", "prompt")
			return p
		}
		p.Detail = questions[0]
		for i := 1; i < len(questions); i++ {
			approvalAppendContextLine(&p.Context, fmt.Sprintf("Question %d: %s", i+1, questions[i]))
		}
		if options := approvalExtractOptions(params, metadata); len(options) > 0 {
			approvalAppendContextLine(&p.Context, "Options: "+strings.Join(options, " | "))
		}
		return p
	default:
		if permissionPresentation, ok := approvalPermissionPresentation(params, raw, metadata); ok {
			return permissionPresentation
		}
		p := approvalPresentation{
			Summary: "approval",
			Context: approvalSharedContextLines(params, nil),
		}
		p.Detail = approvalFirstNonEmptyString(params, nil, "message", "title", "reason", "description", "parsedCmd", "command")
		if p.Detail == "" {
			p.Detail = approvalFirstNonEmptyString(params, nil, "question", "prompt")
		}
		if p.Detail == "" && strings.TrimSpace(method) != "" {
			p.Summary = strings.TrimSpace(method)
		}
		if p.Detail != "" {
			return p
		}
		if len(p.Context) > 0 {
			return p
		}
		if questions := approvalExtractQuestions(params, nil); len(questions) > 0 {
			p.Summary = "user input"
			p.Detail = questions[0]
			for i := 1; i < len(questions); i++ {
				approvalAppendContextLine(&p.Context, fmt.Sprintf("Question %d: %s", i+1, questions[i]))
			}
		}
		return p
	}
}

func approvalPermissionPresentation(params map[string]any, raw map[string]any, metadata map[string]any) (approvalPresentation, bool) {
	permission := strings.ToLower(strings.TrimSpace(approvalFirstNonEmptyString(raw, metadata, "permission", "type", "kind")))
	if permission == "" {
		return approvalPresentation{}, false
	}
	permission = strings.ReplaceAll(permission, "-", "_")
	targets := approvalExtractPaths(raw, params)
	p := approvalPresentation{
		Summary: approvalPermissionSummary(permission),
		Context: approvalSharedContextLines(params, metadata),
	}
	if len(targets) > 0 {
		p.Detail = targets[0]
		for _, target := range targets[1:] {
			approvalAppendContextLine(&p.Context, "Target: "+target)
		}
	}
	if reason := approvalFirstNonEmptyString(params, metadata, "message", "title", "reason", "description"); reason != "" {
		if !strings.EqualFold(reason, p.Detail) {
			if p.Detail == "" {
				p.Detail = reason
			} else {
				approvalAppendContextLine(&p.Context, "Reason: "+reason)
			}
		}
	}
	if p.Detail == "" {
		if question := approvalFirstNonEmptyString(params, metadata, "question", "prompt"); question != "" {
			p.Detail = question
		}
	}
	return p, true
}

func approvalPermissionSummary(permission string) string {
	switch permission {
	case "external_directory":
		return "external directory access"
	case "external_file":
		return "external file access"
	case "external_command":
		return "external command execution"
	case "network", "external_network":
		return "network access"
	default:
		normalized := strings.TrimSpace(strings.ReplaceAll(permission, "_", " "))
		if normalized == "" {
			return "access request"
		}
		return normalized + " access"
	}
}

func approvalFromRecord(record *types.Approval) *ApprovalRequest {
	if record == nil {
		return nil
	}
	params := map[string]any{}
	if len(record.Params) > 0 {
		_ = json.Unmarshal(record.Params, &params)
	}
	presentation := approvalPresentationFromParams(record.Method, params)
	return &ApprovalRequest{
		RequestID: record.RequestID,
		SessionID: record.SessionID,
		Method:    record.Method,
		Summary:   presentation.Summary,
		Detail:    presentation.Detail,
		Context:   cloneStringSlice(presentation.Context),
		CreatedAt: record.CreatedAt,
	}
}

func cloneApprovalRequest(req *ApprovalRequest) *ApprovalRequest {
	if req == nil {
		return nil
	}
	copy := *req
	copy.Context = cloneStringSlice(req.Context)
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
		Context:    cloneStringSlice(req.Context),
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
	copy.Context = cloneStringSlice(resolution.Context)
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
		stringSlicesEqual(left.Context, right.Context) &&
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
		stringSlicesEqual(left.Context, right.Context) &&
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
		if !isApprovalRole(block.Role) {
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
		if !isApprovalRole(block.Role) {
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
		if !isApprovalRole(block.Role) {
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
	for _, line := range req.Context {
		text := strings.TrimSpace(line)
		if text == "" {
			continue
		}
		if len(lines) == 1 && strings.TrimSpace(req.Detail) == "" {
			lines = append(lines, "")
		}
		lines = append(lines, text)
	}
	return ChatBlock{
		ID:        approvalBlockID(req.RequestID),
		Role:      ChatRoleApproval,
		Text:      strings.Join(lines, "\n"),
		Status:    ChatStatusNone,
		CreatedAt: req.CreatedAt,
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
	for _, line := range resolution.Context {
		text := strings.TrimSpace(line)
		if text == "" {
			continue
		}
		if len(lines) == 1 && strings.TrimSpace(resolution.Detail) == "" {
			lines = append(lines, "")
		}
		lines = append(lines, text)
	}
	return ChatBlock{
		ID:        approvalResolutionBlockID(resolution.RequestID),
		Role:      ChatRoleApprovalResolved,
		Text:      strings.Join(lines, "\n"),
		Status:    ChatStatusNone,
		CreatedAt: resolution.ResolvedAt,
		RequestID: resolution.RequestID,
		SessionID: resolution.SessionID,
	}
}

func approvalResolutionBlockID(requestID int) string {
	return fmt.Sprintf("approval:resolved:%d", requestID)
}

func approvalRequestIDFromBlock(block ChatBlock) (int, bool) {
	if isApprovalRole(block.Role) && block.RequestID >= 0 {
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

func isApprovalRole(role ChatRole) bool {
	return role == ChatRoleApproval || role == ChatRoleApprovalResolved
}

func approvalSessionIDFromBlock(block ChatBlock) string {
	return strings.TrimSpace(block.SessionID)
}

func cloneStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		text := strings.TrimSpace(value)
		if text == "" {
			continue
		}
		out = append(out, text)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func stringSlicesEqual(left, right []string) bool {
	left = cloneStringSlice(left)
	right = cloneStringSlice(right)
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func approvalFirstNonEmptyString(primary map[string]any, secondary map[string]any, keys ...string) string {
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if primary != nil {
			if value := strings.TrimSpace(asString(primary[key])); value != "" {
				return value
			}
		}
		if secondary != nil {
			if value := strings.TrimSpace(asString(secondary[key])); value != "" {
				return value
			}
		}
	}
	return ""
}

func approvalExtractQuestions(primary map[string]any, secondary map[string]any) []string {
	var questions []string
	for _, raw := range []any{primary["questions"], primary["prompts"], secondary["questions"], secondary["prompts"]} {
		entries, ok := raw.([]any)
		if !ok {
			continue
		}
		for _, entry := range entries {
			switch typed := entry.(type) {
			case string:
				text := strings.TrimSpace(typed)
				if text == "" {
					continue
				}
				questions = append(questions, text)
			case map[string]any:
				text := approvalFirstNonEmptyString(typed, nil, "text", "prompt", "question", "title", "message")
				if text == "" {
					continue
				}
				questions = append(questions, text)
			}
		}
	}
	return dedupeStrings(questions)
}

func approvalExtractOptions(primary map[string]any, secondary map[string]any) []string {
	var values []string
	for _, container := range []map[string]any{primary, secondary} {
		if container == nil {
			continue
		}
		for _, key := range []string{"options", "choices"} {
			entries, ok := container[key].([]any)
			if !ok {
				continue
			}
			for _, entry := range entries {
				switch typed := entry.(type) {
				case string:
					text := strings.TrimSpace(typed)
					if text == "" {
						continue
					}
					values = append(values, text)
				case map[string]any:
					label := approvalFirstNonEmptyString(typed, nil, "label", "name", "value")
					if label != "" {
						values = append(values, label)
					}
				}
			}
		}
	}
	return dedupeStrings(values)
}

func approvalExtractPaths(primary map[string]any, secondary map[string]any) []string {
	var paths []string
	appendPath := func(raw string) {
		text := strings.TrimSpace(raw)
		if text == "" {
			return
		}
		paths = append(paths, text)
	}
	appendFromArray := func(raw any) {
		entries, ok := raw.([]any)
		if !ok {
			return
		}
		for _, entry := range entries {
			switch typed := entry.(type) {
			case string:
				appendPath(typed)
			case map[string]any:
				appendPath(approvalFirstNonEmptyString(typed, nil, "path", "file", "target"))
			}
		}
	}

	for _, container := range []map[string]any{primary, secondary} {
		if container == nil {
			continue
		}
		appendPath(approvalFirstNonEmptyString(container, nil, "path", "file"))
		appendFromArray(container["paths"])
		appendFromArray(container["files"])
		appendFromArray(container["changes"])
		appendFromArray(container["patterns"])
		appendFromArray(container["always"])
		appendFromArray(container["allow"])
		appendFromArray(container["targets"])
	}
	return dedupeStrings(paths)
}

func approvalSharedContextLines(params map[string]any, metadata map[string]any) []string {
	lines := make([]string, 0, 4)
	if directory := approvalFirstNonEmptyString(params, metadata, "cwd", "directory", "workdir", "workspace"); directory != "" {
		approvalAppendContextLine(&lines, "Directory: "+directory)
	}
	return lines
}

func approvalAppendContextLine(lines *[]string, line string) {
	text := strings.TrimSpace(line)
	if text == "" {
		return
	}
	for _, existing := range *lines {
		if strings.EqualFold(strings.TrimSpace(existing), text) {
			return
		}
	}
	*lines = append(*lines, text)
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		text := strings.TrimSpace(value)
		if text == "" {
			continue
		}
		key := strings.ToLower(text)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, text)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func approvalAsMap(raw any) map[string]any {
	typed, ok := raw.(map[string]any)
	if !ok || typed == nil {
		return nil
	}
	return typed
}

func approvalMergedMap(primary map[string]any, secondary map[string]any) map[string]any {
	switch {
	case len(primary) == 0 && len(secondary) == 0:
		return nil
	case len(primary) == 0:
		return cloneAnyMap(secondary)
	case len(secondary) == 0:
		return cloneAnyMap(primary)
	}
	out := cloneAnyMap(primary)
	for key, value := range secondary {
		if _, exists := out[key]; exists {
			continue
		}
		out[key] = value
	}
	return out
}

func cloneAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]any, len(src))
	for key, value := range src {
		out[key] = value
	}
	return out
}
