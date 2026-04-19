package acp

import (
	"context"
	"encoding/json"
	"fmt"
)

// PermissionHandler handles a typed session/request_permission request from
// the agent. Implementations MUST block until the user decides (allow/deny) or
// the turn is cancelled, and return the outcome the agent should see.
type PermissionHandler func(ctx context.Context, params RequestPermissionParams) (RequestPermissionOutcome, error)

// HandlePermission adapts a PermissionHandler into the generic RequestHandler
// signature accepted by Client.RegisterHandler. The outcome is wrapped in a
// RequestPermissionResult before being sent back to the agent.
func HandlePermission(h PermissionHandler) RequestHandler {
	return func(ctx context.Context, raw json.RawMessage) (any, error) {
		var params RequestPermissionParams
		if err := json.Unmarshal(raw, &params); err != nil {
			return nil, &RPCError{
				Code:    ErrorCodeInvalidParams,
				Message: fmt.Sprintf("invalid session/request_permission params: %v", err),
			}
		}
		outcome, err := h(ctx, params)
		if err != nil {
			return nil, err
		}
		return RequestPermissionResult{Outcome: outcome}, nil
	}
}
