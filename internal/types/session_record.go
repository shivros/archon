package types

type SessionRecord struct {
	Session *Session `json:"session"`
	Source  string   `json:"source"`
}
