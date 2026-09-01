package payment

import (
	"encoding/json"
	"strings"

	"github.com/tokpath/tokstudio/backend/internal/platform/crypto"
)

func specOf(adapter string) (AdapterSpec, bool) {
	a, ok := DefaultRegistry().Get(adapter)
	if !ok {
		return AdapterSpec{}, false
	}
	return a.Spec(), true
}

func missingFromSpec(spec AdapterSpec, creds map[string]string) []string {
	set := map[string]bool{}
	for k, v := range creds {
		if strings.TrimSpace(v) != "" {
			set[k] = true
		}
	}
	return missingRequiredSpec(spec, set)
}

func missingRequiredSpec(spec AdapterSpec, set map[string]bool) []string {
	var missing []string
	for _, f := range spec.Credentials {
		if f.Required && !set[f.Key] {
			missing = append(missing, f.Key)
		}
	}
	return missing
}

func missingRequired(adapter string, set map[string]bool) []string {
	spec, ok := specOf(adapter)
	if !ok {
		return []string{"adapter"}
	}
	return missingRequiredSpec(spec, set)
}

func isSecretCredentialKey(adapter, key string) bool {
	spec, ok := specOf(adapter)
	if ok {
		for _, f := range spec.Credentials {
			if f.Key == key {
				return f.Secret
			}
		}
	}
	switch key {
	case "app_private_key", "alipay_public_key", "merchant_api_private_key", "api_v3_key", "wechat_public_key", "secret_key", "webhook_secret":
		return true
	}
	return strings.Contains(key, "private") || strings.Contains(key, "secret") || strings.Contains(key, "key") && !strings.Contains(key, "public") && key != "app_id" && key != "publishable_key" && key != "public_key_id"
}

func mergeCredentials(existing map[string]string, patch map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range existing {
		out[k] = v
	}
	for k, v := range patch {
		if strings.TrimSpace(v) == "" {
			continue
		}
		out[k] = strings.TrimSpace(v)
	}
	return out
}

func sealCredentials(encKey, adapter string, creds map[string]string) (ciphertext string, meta []byte, err error) {
	raw, err := json.Marshal(creds)
	if err != nil {
		return "", nil, err
	}
	sealed, err := crypto.Seal(encKey, string(raw))
	if err != nil {
		return "", nil, err
	}
	flags := map[string]any{}
	public := map[string]string{}
	for k, v := range creds {
		if strings.TrimSpace(v) == "" {
			continue
		}
		flags[k] = true
		if !isSecretCredentialKey(adapter, k) {
			public[k] = v
		}
	}
	meta, err = json.Marshal(map[string]any{"set": flags, "public": public})
	if err != nil {
		return "", nil, err
	}
	return sealed, meta, nil
}

func openCredentials(encKey, ciphertext string) (map[string]string, error) {
	if strings.TrimSpace(ciphertext) == "" {
		return map[string]string{}, nil
	}
	plain, err := crypto.Open(encKey, ciphertext)
	if err != nil {
		return nil, err
	}
	var creds map[string]string
	if err := json.Unmarshal([]byte(plain), &creds); err != nil {
		return nil, err
	}
	if creds == nil {
		creds = map[string]string{}
	}
	return creds, nil
}

func parseCredMeta(raw []byte) (set map[string]bool, public map[string]string) {
	set = map[string]bool{}
	public = map[string]string{}
	if len(raw) == 0 {
		return set, public
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return set, public
	}
	if m, ok := obj["set"].(map[string]any); ok {
		for k, v := range m {
			if b, ok := v.(bool); ok && b {
				set[k] = true
			}
		}
	}
	if m, ok := obj["public"].(map[string]any); ok {
		for k, v := range m {
			if s, ok := v.(string); ok && s != "" {
				public[k] = s
			}
		}
	}
	return set, public
}

func containsSecretLeak(body string) bool {
	needles := []string{"BEGIN ", "sk_live", "sk_test", "whsec_", "app_private_key", "api_v3_key"}
	for _, n := range needles {
		if strings.Contains(body, n) {
			return true
		}
	}
	return false
}
