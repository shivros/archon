package app

import "strings"

func renderPickerQueryLine(query string) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return " /"
	}
	return " / " + query
}
