package httpx

import (
	"regexp"
	"sort"
	"strings"
	"sync"
)

var messageTemplatesOnce sync.Once
var messageTemplates []struct {
	pattern     *regexp.Regexp
	source      string
	keys        []string
	specificity int
}

// LocalizedSystemMessage is only for trusted system-generated messages.
// Captured names, amounts and authored reasons are interpolated unchanged.
func LocalizedSystemMessage(locale, source string) string {
	if locale == "" || locale == "zh" {
		return source
	}
	messageTemplatesOnce.Do(func() {
		parameter := regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_]*)\}`)
		for key := range uiCatalog {
			normalized := strings.TrimSpace(key)
			matches := parameter.FindAllStringSubmatchIndex(normalized, -1)
			if len(matches) == 0 {
				continue
			}
			pattern, offset, specificity := "(?s)^", 0, 0
			keys := []string{}
			for _, m := range matches {
				literal := normalized[offset:m[0]]
				pattern += regexp.QuoteMeta(literal) + "(.+?)"
				specificity += len([]rune(literal))
				keys = append(keys, normalized[m[2]:m[3]])
				offset = m[1]
			}
			pattern += regexp.QuoteMeta(normalized[offset:]) + "$"
			specificity += len([]rune(normalized[offset:]))
			if specificity < 2 {
				continue
			}
			messageTemplates = append(messageTemplates, struct {
				pattern     *regexp.Regexp
				source      string
				keys        []string
				specificity int
			}{regexp.MustCompile(pattern), key, keys, specificity})
		}
		sort.Slice(messageTemplates, func(i, j int) bool {
			if messageTemplates[i].specificity == messageTemplates[j].specificity {
				return messageTemplates[i].source < messageTemplates[j].source
			}
			return messageTemplates[i].specificity > messageTemplates[j].specificity
		})
	})
	translate := func(text string) (string, bool) {
		if _, ok := uiCatalog[text]; ok {
			return LocalizedText(locale, text), true
		}
		for _, template := range messageTemplates {
			if match := template.pattern.FindStringSubmatch(text); match != nil {
				result := LocalizedText(locale, template.source)
				pairs := []string{}
				for i, key := range template.keys {
					pairs = append(pairs, "{"+key+"}", match[i+1])
				}
				return strings.NewReplacer(pairs...).Replace(result), true
			}
		}
		return text, false
	}
	if result, ok := translate(source); ok {
		return result
	}
	parts := strings.SplitAfter(source, "。")
	for i, part := range parts {
		parts[i], _ = translate(strings.TrimSpace(part))
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}
