package subscriptionauth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
)

const CodexResponsesURL = "https://chatgpt.com/backend-api/codex/responses"

var discoveredModelID = regexp.MustCompile(`^[a-zA-Z0-9_-][a-zA-Z0-9._-]{0,95}(/[a-zA-Z0-9_-][a-zA-Z0-9._-]{0,95}){0,2}$`)

type modelListing struct {
	ID             string `json:"id"`
	Slug           string `json:"slug"`
	SupportedInAPI *bool  `json:"supported_in_api"`
}

// DiscoverModels reads the authorized account's model catalog. A connection
// may omit this API; authorization and existing routes still work.
func (c Connection) DiscoverModels(ctx context.Context, grant Credential, guard DestinationGuard) []string {
	base, ok := c.inferenceBase()
	if !ok || guard == nil || grant.Connection != c.Fingerprint() || !safeToken(grant.AccessToken) {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var account string
	if c.InferenceURL == CodexResponsesURL {
		account = CodexRoutingAccount(grant.AccessToken)
		if account == "" {
			return nil
		}
	}
	var models []string
	for _, model := range fetchModelListing(ctx, base+"/models", grant.AccessToken, account, guard) {
		if model.SupportedInAPI != nil && !*model.SupportedInAPI {
			continue
		}
		id := model.ID
		if id == "" {
			id = model.Slug
		}
		if discoveredModelID.MatchString(id) {
			models = append(models, id)
		}
	}
	slices.Sort(models)
	models = slices.Compact(models)
	return models[:min(len(models), 100)]
}

// Derive the listing endpoint from the approved inference origin and prefix.
func (c Connection) inferenceBase() (string, bool) {
	u, err := url.Parse(c.InferenceURL)
	if err != nil || u.Host == "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.User != nil {
		return "", false
	}
	for _, suffix := range []string{"/responses", "/chat/completions", "/messages"} {
		if base, ok := strings.CutSuffix(c.InferenceURL, suffix); ok {
			return base, true
		}
	}
	return "", false
}

func fetchModelListing(ctx context.Context, endpoint, token, account string, guard DestinationGuard) []modelListing {
	if guard(ctx, endpoint) != nil {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if account != "" {
		req.Header.Set("ChatGPT-Account-Id", account)
	}
	resp, err := refreshClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var list struct {
		Data   []modelListing `json:"data"`
		Models []modelListing `json:"models"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&list) != nil {
		return nil
	}
	return append(list.Data, list.Models...)
}
