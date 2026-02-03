package types

type LogEvent struct {
	Type   string `json:"type"`
	Stream string `json:"stream"`
	Chunk  string `json:"chunk"`
	TS     string `json:"ts"`
}
