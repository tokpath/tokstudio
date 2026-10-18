package portal

import (
	"net/url"
	"strings"
)

// safeCustomerNext carries an existing customer task through the verification
// email without letting a caller choose another origin or an arbitrary route.
func safeCustomerNext(raw string) string {
	if raw == "" || len(raw) > 2048 || strings.IndexFunc(raw, func(r rune) bool { return r <= ' ' || r == 127 || r == '\\' || r == '#' }) >= 0 {
		return ""
	}
	target, err := url.ParseRequestURI(raw)
	if err != nil || target.IsAbs() || target.Host != "" || target.EscapedPath() != target.Path {
		return ""
	}
	var keys []string
	switch target.Path {
	case "/workspace/models", "/workspace/playground":
		keys = []string{"model"}
	case "/workspace/plans":
		keys = []string{"plan", "kol_activity"}
	default:
		return ""
	}
	query, err := url.ParseQuery(target.RawQuery)
	if err != nil {
		return ""
	}
	parts := []string{}
	for _, key := range keys {
		value := query.Get(key)
		if value != "" && strings.IndexFunc(value, func(r rune) bool { return r < ' ' || r == 127 || r == '\\' }) < 0 {
			parts = append(parts, key+"="+url.QueryEscape(value))
		}
	}
	if len(parts) == 0 {
		return target.Path
	}
	return target.Path + "?" + strings.Join(parts, "&")
}
