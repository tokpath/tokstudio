package httpx

import (
	_ "embed"
	"encoding/json"
)

//go:embed error_i18n.json
var errorCatalogJSON []byte

var errorCatalog struct {
	Codes    map[string][2]string `json:"codes"`
	Messages map[string][2]string `json:"messages"`
}

func init() {
	if err := json.Unmarshal(errorCatalogJSON, &errorCatalog); err != nil {
		panic(err)
	}
}

func localizedErrorMessage(locale, code, message string) string {
	if locale != "en" && locale != "ja" {
		return message
	}
	index := 0
	if locale == "ja" {
		index = 1
	}
	if text, ok := errorCatalog.Messages[message]; ok {
		return text[index]
	}
	if text, ok := errorCatalog.Codes[code]; ok {
		return text[index]
	}
	return errorCatalog.Codes["internal_error"][index]
}
