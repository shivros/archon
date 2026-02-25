package types

type DebugEvent struct {
	Type      string `json:"type"`
	SessionID string `json:"session_id"`
	Provider  string `json:"provider"`
	Stream    string `json:"stream"`
	Chunk     string `json:"chunk"`
	TS        string `json:"ts"`
	Seq       uint64 `json:"seq"`
}
