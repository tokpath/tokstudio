package httpx

import (
	_ "embed"
	"encoding/json"
	"regexp"
	"strings"
)

//go:embed error_i18n.json
var errorCatalogJSON []byte

//go:embed ui_i18n.generated.json
var uiCatalogJSON []byte

var uiCatalog map[string][4]string

var errorCatalog struct {
	Codes    map[string][3]string `json:"codes"`
	Messages map[string][3]string `json:"messages"`
}

func init() {
	if err := json.Unmarshal(errorCatalogJSON, &errorCatalog); err != nil {
		panic(err)
	}
	if err := json.Unmarshal(uiCatalogJSON, &uiCatalog); err != nil {
		panic(err)
	}
}

// LocalizedText accepts marked system copy only. Authored names, evidence and
// announcements must not be passed here. Unknown text remains unchanged.
func LocalizedText(locale, source string) string {
	index := localeIndex(locale) + 1
	if value, ok := uiCatalog[source]; ok {
		return value[index]
	}
	for _, template := range requirementTemplates {
		if match := template.pattern.FindStringSubmatch(source); match != nil {
			message, ok := uiCatalog[template.source]
			if !ok || message[index] == "" {
				return source
			}
			text := message[index]
			for i, key := range template.keys {
				text = strings.ReplaceAll(text, "{"+key+"}", match[i+1])
			}
			return text
		}
	}
	return source
}

// Only domain-generated closure balances/counts use dynamic rendering here.
// Amount strings are never parsed or rounded, and authored content is excluded.
var requirementTemplates = []struct {
	pattern *regexp.Regexp
	source  string
	keys    []string
}{
	{regexp.MustCompile(`^frozen commission 结算币种: (-?[0-9.]+)$`), "frozen commission 结算币种: {amount}", []string{"amount"}},
	{regexp.MustCompile(`^pending commission recovery 结算币种: (-?[0-9.]+)$`), "pending commission recovery 结算币种: {amount}", []string{"amount"}},
	{regexp.MustCompile(`^available/held commission 结算币种: (-?[0-9.]+) / (-?[0-9.]+)$`), "available/held commission 结算币种: {available} / {held}", []string{"available", "held"}},
	{regexp.MustCompile(`^([0-9]+) actual original-currency receipts await verified valuation or receipt matching; ([0-9]+) recorded receipts retain unallocated principal$`), "{receipts} actual original-currency receipts await verified valuation or receipt matching; {recorded} recorded receipts retain unallocated principal", []string{"receipts", "recorded"}},
}

func localeIndex(locale string) int {
	switch locale {
	case "en":
		return 0
	case "ja":
		return 1
	case "ru":
		return 2
	default:
		return -1
	}
}

func localizedErrorMessage(locale, code, message string) string {
	return LocalizedErrorMessage(locale, code, message)
}

// LocalizedErrorMessage changes presentation only; callers keep error codes,
// retryability, request IDs and operation state unchanged.
func LocalizedErrorMessage(locale, code, message string) string {
	index := localeIndex(locale)
	if text, ok := uiCatalog[message]; ok {
		return text[index+1]
	}
	if tail, ok := strings.CutPrefix(message, "仍有未满足事项："); ok {
		parts := strings.Split(tail, "；")
		for i, part := range parts {
			parts[i] = LocalizedText(locale, part)
		}
		return LocalizedText(locale, "仍有未满足事项：") + strings.Join(parts, "\n")
	}
	if index < 0 {
		return LocalizedText(locale, message)
	}
	if text, ok := errorCatalog.Messages[message]; ok {
		return text[index]
	}
	if text, ok := errorCatalog.Codes[code]; ok {
		return text[index]
	}
	return errorCatalog.Codes["internal_error"][index]
}
