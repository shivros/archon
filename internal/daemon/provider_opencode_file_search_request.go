package daemon

import "strings"

type openCodeFileSearchRequest struct {
	Query     string
	Directory string
	Limit     int
}

func normalizeOpenCodeFileSearchRequest(req openCodeFileSearchRequest) openCodeFileSearchRequest {
	req.Query = strings.TrimSpace(req.Query)
	req.Directory = strings.TrimSpace(req.Directory)
	return req
}
