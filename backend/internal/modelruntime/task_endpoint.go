package modelruntime

import (
	"net/url"
	"strings"
)

// ResolveTaskEndpoint substitutes only a complete path segment in an endpoint
// configured by the platform. Provider task IDs cannot change the authority,
// query, or surrounding resource path. The normal egress guard still applies.
func ResolveTaskEndpoint(template, taskID string) (string, error) {
	u, err := url.Parse(template)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Count(template, "{task_id}") != 1 || strings.ContainsAny(taskID, "\x00\r\n") || taskID == "" || len(taskID) > 2048 || taskID == "." || taskID == ".." {
		return "", ErrInvalidRequest
	}
	if strings.ContainsAny(strings.Replace(template, "{task_id}", "", 1), "{}") {
		return "", ErrInvalidRequest
	}
	found := false
	for _, part := range strings.Split(u.Path, "/") {
		if part == "." || part == ".." {
			return "", ErrInvalidRequest
		}
		if part == "{task_id}" {
			found = true
		}
	}
	if !found {
		return "", ErrInvalidRequest
	}
	return strings.Replace(template, "{task_id}", url.PathEscape(taskID), 1), nil
}
