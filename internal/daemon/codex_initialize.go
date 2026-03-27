package daemon

import "strings"

type codexInitializeOptions struct {
	ClientName      string
	ClientTitle     string
	ClientVersion   string
	ExperimentalAPI bool
}

func codexInitializeParams(opts codexInitializeOptions) map[string]any {
	params := map[string]any{
		"clientInfo": map[string]string{
			"name":    defaultCodexInitializeString(opts.ClientName, "archon"),
			"title":   defaultCodexInitializeString(opts.ClientTitle, "Archon"),
			"version": defaultCodexInitializeString(opts.ClientVersion, "dev"),
		},
	}
	if opts.ExperimentalAPI {
		params["capabilities"] = map[string]any{
			"experimentalApi": true,
		}
	}
	return params
}

func defaultCodexInitializeString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}
