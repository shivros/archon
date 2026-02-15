package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"control/internal/logging"
	"control/internal/providers"
	"control/internal/store"
	"control/internal/types"
)

type ApprovalSyncProvider interface {
	Provider() string
	SyncSessionApprovals(ctx context.Context, session *types.Session, meta *types.SessionMeta) (*ApprovalSyncResult, error)
}

type ApprovalSyncResult struct {
	Approvals     []*types.Approval
	Authoritative bool
}

type ApprovalResyncService struct {
	stores    *Stores
	logger    logging.Logger
	providers map[string]ApprovalSyncProvider
}

func NewApprovalResyncService(stores *Stores, logger logging.Logger, extra ...ApprovalSyncProvider) *ApprovalResyncService {
	if logger == nil {
		logger = logging.Nop()
	}
	service := &ApprovalResyncService{
		stores:    stores,
		logger:    logger,
		providers: map[string]ApprovalSyncProvider{},
	}
	service.registerProvider(&codexApprovalSyncProvider{stores: stores, logger: logger})
	service.registerProvider(&openCodeApprovalSyncProvider{provider: "opencode", stores: stores, logger: logger})
	service.registerProvider(&openCodeApprovalSyncProvider{provider: "kilocode", stores: stores, logger: logger})
	for _, provider := range extra {
		service.registerProvider(provider)
	}
	return service
}

func (s *ApprovalResyncService) registerProvider(provider ApprovalSyncProvider) {
	if s == nil || provider == nil {
		return
	}
	name := providers.Normalize(provider.Provider())
	if name == "" {
		return
	}
	s.providers[name] = provider
}

func (s *ApprovalResyncService) SyncAll(ctx context.Context) error {
	if s == nil || s.stores == nil || s.stores.Sessions == nil {
		return nil
	}
	records, err := s.stores.Sessions.ListRecords(ctx)
	if err != nil {
		return err
	}
	metaBySessionID := map[string]*types.SessionMeta{}
	if s.stores.SessionMeta != nil {
		entries, metaErr := s.stores.SessionMeta.List(ctx)
		if metaErr != nil {
			return metaErr
		}
		for _, entry := range entries {
			if entry == nil || strings.TrimSpace(entry.SessionID) == "" {
				continue
			}
			metaBySessionID[entry.SessionID] = entry
		}
	}

	var firstErr error
	for _, record := range records {
		if record == nil || record.Session == nil {
			continue
		}
		session := record.Session
		meta := metaBySessionID[session.ID]
		if err := s.SyncSession(ctx, session, meta); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			if s.logger != nil {
				s.logger.Warn("approval_resync_failed",
					logging.F("session_id", session.ID),
					logging.F("provider", session.Provider),
					logging.F("error", err),
				)
			}
		}
	}
	return firstErr
}

func (s *ApprovalResyncService) SyncSession(ctx context.Context, session *types.Session, meta *types.SessionMeta) error {
	if s == nil || session == nil || strings.TrimSpace(session.ID) == "" {
		return nil
	}
	if s.stores == nil || s.stores.Approvals == nil {
		return nil
	}
	providerName := providers.Normalize(session.Provider)
	provider, ok := s.providers[providerName]
	if !ok || provider == nil {
		return nil
	}
	result, err := provider.SyncSessionApprovals(ctx, session, meta)
	if err != nil {
		return err
	}
	if result == nil {
		return nil
	}
	return s.reconcileSessionApprovals(ctx, session.ID, result.Approvals, result.Authoritative)
}

func (s *ApprovalResyncService) reconcileSessionApprovals(ctx context.Context, sessionID string, pending []*types.Approval, authoritative bool) error {
	if s == nil || s.stores == nil || s.stores.Approvals == nil {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	existing, err := s.stores.Approvals.ListBySession(ctx, sessionID)
	if err != nil {
		return err
	}
	pendingByRequestID := map[int]*types.Approval{}
	for _, approval := range pending {
		if approval == nil || approval.RequestID < 0 {
			continue
		}
		copy := *approval
		copy.SessionID = sessionID
		pendingByRequestID[copy.RequestID] = &copy
	}

	for _, approval := range pendingByRequestID {
		if _, err := s.stores.Approvals.Upsert(ctx, approval); err != nil {
			return err
		}
	}
	if authoritative {
		for _, approval := range existing {
			if approval == nil {
				continue
			}
			if _, keep := pendingByRequestID[approval.RequestID]; keep {
				continue
			}
			if err := s.stores.Approvals.Delete(ctx, sessionID, approval.RequestID); err != nil && !errors.Is(err, store.ErrApprovalNotFound) {
				return err
			}
		}
	}
	return nil
}

type codexApprovalSyncProvider struct {
	stores  *Stores
	logger  logging.Logger
	timeout time.Duration
}

type openCodeApprovalSyncProvider struct {
	provider string
	stores   *Stores
	logger   logging.Logger
	timeout  time.Duration
}

func (p *openCodeApprovalSyncProvider) Provider() string {
	return p.provider
}

func (p *openCodeApprovalSyncProvider) SyncSessionApprovals(ctx context.Context, session *types.Session, meta *types.SessionMeta) (*ApprovalSyncResult, error) {
	if session == nil {
		return nil, nil
	}
	if providers.Normalize(session.Provider) != providers.Normalize(p.provider) {
		return nil, nil
	}
	providerSessionID := ""
	if meta != nil {
		providerSessionID = strings.TrimSpace(meta.ProviderSessionID)
	}
	if providerSessionID == "" {
		return nil, nil
	}

	timeout := p.timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	client, err := newOpenCodeClient(resolveOpenCodeClientConfig(session.Provider, loadCoreConfigOrDefault()))
	if err != nil {
		return nil, err
	}
	permissions, err := client.ListPermissions(callCtx, providerSessionID, session.Cwd)
	if err != nil {
		return nil, err
	}

	usedIDs := map[int]string{}
	approvals := make([]*types.Approval, 0, len(permissions))
	for _, permission := range permissions {
		if permission.Status != "" && permission.Status != "pending" {
			continue
		}
		if permission.SessionID != "" && permission.SessionID != providerSessionID {
			continue
		}
		method := openCodePermissionMethod(permission)
		params := openCodeApprovalParams(permission, method)
		paramsRaw, _ := json.Marshal(params)
		requestID := openCodePermissionRequestID(permission.PermissionID, usedIDs)
		approvals = append(approvals, &types.Approval{
			SessionID: session.ID,
			RequestID: requestID,
			Method:    method,
			Params:    paramsRaw,
			CreatedAt: permission.CreatedAt,
		})
	}
	sort.Slice(approvals, func(i, j int) bool {
		if approvals[i].CreatedAt.Equal(approvals[j].CreatedAt) {
			return approvals[i].RequestID < approvals[j].RequestID
		}
		return approvals[i].CreatedAt.Before(approvals[j].CreatedAt)
	})
	return &ApprovalSyncResult{
		Approvals:     approvals,
		Authoritative: true,
	}, nil
}

func (p *codexApprovalSyncProvider) Provider() string {
	return "codex"
}

func (p *codexApprovalSyncProvider) SyncSessionApprovals(ctx context.Context, session *types.Session, meta *types.SessionMeta) (*ApprovalSyncResult, error) {
	if session == nil {
		return nil, nil
	}
	threadID := strings.TrimSpace(resolveThreadID(session, meta))
	if threadID == "" {
		return nil, nil
	}
	cwd := strings.TrimSpace(session.Cwd)
	if cwd == "" {
		return nil, nil
	}
	workspacePath := ""
	if meta != nil && p.stores != nil && p.stores.Workspaces != nil && strings.TrimSpace(meta.WorkspaceID) != "" {
		if ws, ok, err := p.stores.Workspaces.Get(ctx, strings.TrimSpace(meta.WorkspaceID)); err == nil && ok && ws != nil {
			workspacePath = ws.RepoPath
		}
	}
	codexHome := resolveCodexHome(cwd, workspacePath)
	timeout := p.timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	syncCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	client, err := startCodexAppServer(syncCtx, cwd, codexHome, p.logger)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	thread, err := client.ReadThread(syncCtx, threadID)
	if err != nil {
		return nil, err
	}
	items := flattenCodexItems(thread)
	approvals, authoritative := extractPendingApprovalsFromCodexItems(items, session.ID)
	return &ApprovalSyncResult{
		Approvals:     approvals,
		Authoritative: authoritative,
	}, nil
}

func extractPendingApprovalsFromCodexItems(items []map[string]any, sessionID string) ([]*types.Approval, bool) {
	if len(items) == 0 {
		return nil, false
	}
	pendingByID := map[int]*types.Approval{}
	seenSignal := false
	now := time.Now().UTC()
	for i, item := range items {
		if item == nil {
			continue
		}
		if approval := approvalFromCodexItem(item, sessionID); approval != nil {
			seenSignal = true
			if approval.CreatedAt.IsZero() {
				approval.CreatedAt = now.Add(time.Duration(i) * time.Microsecond)
			}
			pendingByID[approval.RequestID] = approval
			continue
		}
		if requestID, ok := resolvedApprovalRequestIDFromCodexItem(item); ok {
			seenSignal = true
			delete(pendingByID, requestID)
		}
	}
	if len(pendingByID) == 0 {
		return nil, false
	}
	out := make([]*types.Approval, 0, len(pendingByID))
	for _, approval := range pendingByID {
		copy := *approval
		if copy.CreatedAt.IsZero() {
			copy.CreatedAt = now
		}
		out = append(out, &copy)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].RequestID < out[j].RequestID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, seenSignal
}

func approvalFromCodexItem(item map[string]any, sessionID string) *types.Approval {
	method := approvalMethodFromCodexItem(item)
	if method == "" {
		return nil
	}
	requestID, ok := approvalRequestIDFromCodexItem(item)
	if !ok || requestID < 0 {
		return nil
	}
	params := approvalParamsFromCodexItem(item)
	paramsRaw, _ := json.Marshal(params)
	return &types.Approval{
		SessionID: sessionID,
		RequestID: requestID,
		Method:    method,
		Params:    paramsRaw,
		CreatedAt: approvalTimestampFromCodexItem(item),
	}
}

func approvalMethodFromCodexItem(item map[string]any) string {
	method := strings.TrimSpace(strings.ToLower(asString(item["method"])))
	switch method {
	case "item/commandexecution/requestapproval":
		return "item/commandExecution/requestApproval"
	case "item/filechange/requestapproval":
		return "item/fileChange/requestApproval"
	case "tool/requestuserinput":
		return "tool/requestUserInput"
	}
	typ := strings.TrimSpace(strings.ToLower(asString(item["type"])))
	switch typ {
	case "item/commandexecution/requestapproval":
		return "item/commandExecution/requestApproval"
	case "item/filechange/requestapproval":
		return "item/fileChange/requestApproval"
	case "tool/requestuserinput":
		return "tool/requestUserInput"
	}
	status := strings.TrimSpace(strings.ToLower(asString(item["status"])))
	switch typ {
	case "commandexecution":
		if hasApprovalRequiredFlag(item) || isApprovalRequiredStatus(status) {
			return "item/commandExecution/requestApproval"
		}
	case "filechange":
		if hasApprovalRequiredFlag(item) || isApprovalRequiredStatus(status) {
			return "item/fileChange/requestApproval"
		}
	case "requestuserinput", "userinputrequest":
		return "tool/requestUserInput"
	}
	return ""
}

func hasApprovalRequiredFlag(item map[string]any) bool {
	for _, key := range []string{"approvalRequired", "approval_required", "requiresApproval", "requires_approval"} {
		if val, ok := item[key]; ok && asBool(val) {
			return true
		}
	}
	return false
}

func isApprovalRequiredStatus(status string) bool {
	switch status {
	case "approval_required", "needs_approval", "pending_approval", "awaiting_approval", "waiting_approval":
		return true
	default:
		return false
	}
}

func approvalRequestIDFromCodexItem(item map[string]any) (int, bool) {
	keys := []string{"requestId", "request_id", "approvalRequestId", "approval_request_id", "id"}
	for _, key := range keys {
		if requestID, ok := asInt(item[key]); ok && requestID >= 0 {
			return requestID, true
		}
	}
	for _, key := range []string{"params", "request", "approval", "result"} {
		nested, ok := item[key].(map[string]any)
		if !ok || nested == nil {
			continue
		}
		for _, nestedKey := range keys {
			if requestID, ok := asInt(nested[nestedKey]); ok && requestID >= 0 {
				return requestID, true
			}
		}
	}
	return 0, false
}

func resolvedApprovalRequestIDFromCodexItem(item map[string]any) (int, bool) {
	method := strings.ToLower(strings.TrimSpace(asString(item["method"])))
	if strings.Contains(method, "requestapproval") {
		return 0, false
	}
	requestID, ok := approvalRequestIDFromCodexItem(item)
	if !ok {
		return 0, false
	}
	if hasDecision(item) {
		return requestID, true
	}
	params, _ := item["params"].(map[string]any)
	if hasDecision(params) {
		return requestID, true
	}
	return 0, false
}

func hasDecision(item map[string]any) bool {
	if item == nil {
		return false
	}
	decision := strings.TrimSpace(strings.ToLower(asString(item["decision"])))
	switch decision {
	case "accept", "accepted", "approve", "approved", "decline", "declined", "reject", "rejected":
		return true
	}
	return false
}

func approvalParamsFromCodexItem(item map[string]any) map[string]any {
	if item == nil {
		return map[string]any{}
	}
	params, ok := item["params"].(map[string]any)
	if ok && params != nil {
		copy := map[string]any{}
		for key, value := range params {
			copy[key] = value
		}
		return copy
	}
	copy := map[string]any{}
	for _, key := range []string{"parsedCmd", "command", "reason", "questions"} {
		if value, ok := item[key]; ok {
			copy[key] = value
		}
	}
	return copy
}

func approvalTimestampFromCodexItem(item map[string]any) time.Time {
	if item == nil {
		return time.Time{}
	}
	for _, key := range []string{"ts", "timestamp", "createdAt", "created_at"} {
		raw, ok := item[key]
		if !ok {
			continue
		}
		if parsed := parseApprovalTimestamp(raw); !parsed.IsZero() {
			return parsed
		}
	}
	return time.Time{}
}

func parseApprovalTimestamp(raw any) time.Time {
	switch val := raw.(type) {
	case string:
		text := strings.TrimSpace(val)
		if text == "" {
			return time.Time{}
		}
		if parsed, err := time.Parse(time.RFC3339Nano, text); err == nil {
			return parsed.UTC()
		}
		if parsed, err := strconv.ParseInt(text, 10, 64); err == nil {
			return time.Unix(parsed, 0).UTC()
		}
	case float64:
		return time.Unix(int64(val), 0).UTC()
	case int:
		return time.Unix(int64(val), 0).UTC()
	case int64:
		return time.Unix(val, 0).UTC()
	case json.Number:
		if parsed, err := val.Int64(); err == nil {
			return time.Unix(parsed, 0).UTC()
		}
	}
	return time.Time{}
}

func asBool(raw any) bool {
	switch val := raw.(type) {
	case bool:
		return val
	case string:
		switch strings.TrimSpace(strings.ToLower(val)) {
		case "1", "true", "yes", "on":
			return true
		}
	case int:
		return val != 0
	case int64:
		return val != 0
	case float64:
		return val != 0
	}
	return false
}

func asString(raw any) string {
	if raw == nil {
		return ""
	}
	switch val := raw.(type) {
	case string:
		return val
	case json.Number:
		return val.String()
	default:
		return ""
	}
}

func asInt(raw any) (int, bool) {
	switch val := raw.(type) {
	case int:
		return val, true
	case int32:
		return int(val), true
	case int64:
		return int(val), true
	case float64:
		return int(val), true
	case json.Number:
		if parsed, err := val.Int64(); err == nil {
			return int(parsed), true
		}
	case string:
		text := strings.TrimSpace(val)
		if text == "" {
			return 0, false
		}
		parsed, err := strconv.Atoi(text)
		if err != nil {
			return 0, false
		}
		return parsed, true
	}
	return 0, false
}
