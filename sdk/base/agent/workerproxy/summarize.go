package workerproxy

import "strings"

func summarizeBody(body []byte, limit int) string {
	if limit <= 0 || len(body) == 0 {
		return ""
	}
	s := strings.TrimSpace(string(body))
	if len(s) <= limit {
		return s
	}
	if limit <= 3 {
		return s[:limit]
	}
	return s[:limit-3] + "..."
}
