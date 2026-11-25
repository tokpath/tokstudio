package subscriptionauth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"sort"
	"time"
)

type DiscoveredModel struct {
	ID           string
	Capabilities []string
}

type modelListing struct {
	ID              string          `json:"id"`
	Slug            string          `json:"slug"`
	SupportedInAPI  *bool           `json:"supported_in_api"`
	Capabilities    json.RawMessage `json:"capabilities"`
	InputModalities []string        `json:"input_modalities"`
}

// DiscoverModels reads the authorized account's model catalog. A connection
// may omit this API; authorization and existing routes still work.
func (c Connection) DiscoverModels(ctx context.Context, grant Credential, guard DestinationGuard) []DiscoveredModel {
	base, ok := c.inferenceBase()
	if !ok || guard == nil || grant.Connection != c.Fingerprint() || !safeToken(grant.AccessToken) {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var account string
	if NativeCodexEndpoint(c.InferenceURL) {
		account = CodexRoutingAccount(grant.AccessToken)
		if account == "" {
			return nil
		}
	}
	type listingSource struct{ path, operation string }
	sources := []listingSource{{path: "/models"}}
	if c.MediaPreset == "xai" {
		sources = append(sources, listingSource{"/image-generation-models", ImageGeneration},
			listingSource{"/video-generation-models", VideoGeneration})
	}
	models := make(map[string]DiscoveredModel)
	for _, source := range sources {
		for _, model := range fetchModelListing(ctx, base+source.path, grant.AccessToken, account, guard) {
			if model.SupportedInAPI != nil && !*model.SupportedInAPI {
				continue
			}
			id := model.ID
			if id == "" {
				id = model.Slug
			}
			if !discoveredModelID.MatchString(id) {
				continue
			}
			var capabilities []string
			_ = json.Unmarshal(model.Capabilities, &capabilities)
			saved := models[id]
			saved.ID = id
			saved.Capabilities = append(saved.Capabilities, capabilities...)
			if source.operation != "" {
				saved.Capabilities = append(saved.Capabilities, source.operation)
			}
			if source.operation == ImageGeneration && slices.Contains(model.InputModalities, "image") {
				saved.Capabilities = append(saved.Capabilities, ImageEdit)
			}
			models[id] = saved
		}
	}
	result := make([]DiscoveredModel, 0, len(models))
	for _, model := range models {
		result = append(result, model)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result[:min(len(result), 100)]
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
