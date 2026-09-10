package payment

import "strings"

func officialLive(mode string, creds map[string]string, keys ...string) bool {
	if strings.TrimSpace(mode) != ModeLive {
		return false
	}
	for _, k := range keys {
		if strings.TrimSpace(creds[k]) == "" {
			return false
		}
	}
	return true
}

func cred(creds map[string]string, key string) string {
	if creds == nil {
		return ""
	}
	return strings.TrimSpace(creds[key])
}

func stripeCents(amountMinor int64) int64 {
	if amountMinor <= 0 {
		return 0
	}
	cents := amountMinor / 10000
	if cents <= 0 {
		return 1
	}
	return cents
}
