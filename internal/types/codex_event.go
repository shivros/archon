package types

import "encoding/json"

type CodexEvent struct {
	ID     *int            `json:"id,omitempty"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
	TS     string          `json:"ts,omitempty"`
}
