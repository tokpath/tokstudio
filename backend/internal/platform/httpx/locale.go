package httpx

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// RequestLocale follows the web UI: an explicit cookie overrides Accept-Language.
func RequestLocale(r *http.Request) string {
	if r == nil {
		return "zh"
	}
	if cookie, err := r.Cookie("NEXT_LOCALE"); err == nil {
		if locale := supportedLocale(cookie.Value); locale != "" {
			return locale
		}
	}
	type preference struct {
		tag string
		q   float64
	}
	var preferences []preference
	for _, item := range strings.Split(r.Header.Get("Accept-Language"), ",") {
		parts := strings.Split(item, ";")
		p := preference{tag: strings.ToLower(strings.TrimSpace(parts[0])), q: 1}
		for _, part := range parts[1:] {
			if strings.HasPrefix(strings.TrimSpace(part), "q=") {
				q, err := strconv.ParseFloat(strings.TrimPrefix(strings.TrimSpace(part), "q="), 64)
				if err != nil || q < 0 || q > 1 {
					q = 0
				}
				p.q = q
			}
		}
		if p.q > 0 {
			preferences = append(preferences, p)
		}
	}
	sort.SliceStable(preferences, func(i, j int) bool { return preferences[i].q > preferences[j].q })
	for _, p := range preferences {
		if locale := supportedLocale(p.tag); locale != "" {
			return locale
		}
	}
	return "zh"
}

func supportedLocale(value string) string {
	language := strings.SplitN(strings.ReplaceAll(strings.ToLower(strings.TrimSpace(value)), "_", "-"), "-", 2)[0]
	switch language {
	case "zh", "en", "ja", "ru":
		return language
	default:
		return ""
	}
}
