package types

import (
	"encoding/json"
	"time"
)

type Approval struct {
	SessionID string          `json:"session_id"`
	RequestID int             `json:"request_id"`
	Method    string          `json:"method"`
	Params    json.RawMessage `json:"params,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}
