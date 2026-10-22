package catalog

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
	platformcrypto "github.com/tokpath/tokstudio/backend/internal/platform/crypto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

//go:embed commerce_schema.sql
var commerceSchema string

//go:embed commerce_schema_02_credential_name.sql
var credentialNameSchema string

//go:embed commerce_schema.sql
//go:embed commerce_schema_02_credential_name.sql
//go:embed commerce_schema_03_supplier_models.sql
//go:embed commerce_schema_04_capability_fields.sql
var commerceMigrationFS embed.FS

//go:embed commerce_schema_03_supplier_models.sql
var supplierModelSchema string

//go:embed commerce_schema_04_capability_fields.sql
var capabilityFieldSchema string

func CommerceMigrations() (string, fs.FS) { return "catalog_v2", commerceMigrationFS }

func CommerceSchema() string {
	return commerceSchema + "\n" + credentialNameSchema + "\n" + supplierModelSchema + "\n" + capabilityFieldSchema
}

var ErrCatalogForbidden = errors.New("catalog action outside operator authority")
var ErrCatalogVersion = errors.New("catalog version conflict")
var ErrInvalidInput = errors.New("invalid catalog input")
var ErrModelNotVisible = errors.New("model not visible to operator")
var ErrPublicRoute = errors.New("public capability has no verified healthy upstream")
var ErrPublicCapability = errors.New("public model has no capability")

type CatalogOperator struct{ ID, BrandID, ParentID, Kind string }
type OperatorDirectory interface {
	CatalogOperator(context.Context, string) (CatalogOperator, error)
}

// CommerceCatalog is the new brand/operator catalog. Caller identities must be
// obtained from authenticated server context; JSON operator IDs are never actors.
type CommerceCatalog struct {
	DB                 *gorm.DB
	Operators          OperatorDirectory
	PlatformOperatorID string
	EncryptionKey      string
}
type PublicModel struct {
	ID           string                    `json:"id"`
	Identity     string                    `json:"identity"` // platform-attested model identity/revision
	DisplayName  string                    `json:"display_name"`
	Enabled      bool                      `json:"enabled"`
	Capabilities []modelruntime.Capability `json:"capabilities"`
	Version      uint64                    `json:"version"`
}
type UpstreamModel struct {
	ID            string `json:"id"`
	PublicModelID string `json:"public_model_id"`
	Identity      string `json:"identity"`
	ProviderID    string `json:"provider_id"`
	Provider      string `json:"provider"`
	Model         string `json:"model"`
	CredentialID  string `json:"credential_id"`
	// Endpoints are exact verified URLs, not customer-controlled base URLs.
	Endpoints       map[modelruntime.Operation]string `json:"endpoints,omitempty"`
	Capabilities    []modelruntime.Capability         `json:"capabilities"`
	Enabled         bool                              `json:"enabled"`
	Healthy         bool                              `json:"healthy"`
	Priority        int                               `json:"priority"`
	Routing         RouteLimits                       `json:"routing"`
	SupplierModelID string                            `json:"supplier_model_id,omitempty"`
	Version         uint64                            `json:"version"`
}
type CredentialView struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ProviderID  string `json:"provider_id"`
	Fingerprint string `json:"fingerprint"`
	Enabled     bool   `json:"enabled"`
	Version     uint64 `json:"version"`
}
type catalogJSONRow struct {
	ID         string
	Definition []byte
	Version    uint64
}

func (s CommerceCatalog) platform(actor string) bool {
	return actor != "" && s.PlatformOperatorID != "" && actor == s.PlatformOperatorID
}
func (s CommerceCatalog) WithDB(db *gorm.DB) CommerceCatalog { s.DB = db; return s }

// LockRevision must be called inside the admission/coverage transaction. The
// shared row lock remains held through reservations. Every catalog mutation
// holds an exclusive lock on the same row and advances its version atomically.
func (s CommerceCatalog) LockRevision(ctx context.Context) (uint64, error) {
	var row struct{ Version uint64 }
	e := s.DB.WithContext(ctx).Table("tokenhub_catalog_revisions").Clauses(clause.Locking{Strength: "SHARE"}).Where("id='global'").Take(&row).Error
	return row.Version, e
}
func (s CommerceCatalog) mutate(ctx context.Context, fn func(CommerceCatalog) error) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row struct{ Version uint64 }
		if e := tx.Table("tokenhub_catalog_revisions").Clauses(clause.Locking{Strength: "UPDATE"}).Where("id='global'").Take(&row).Error; e != nil {
			return e
		}
		if e := fn(s.WithDB(tx)); e != nil {
			return e
		}
		return tx.Table("tokenhub_catalog_revisions").Where("id='global'").Update("version", row.Version+1).Error
	})
}
func (s CommerceCatalog) SavePublicModel(ctx context.Context, actor string, in PublicModel, expected uint64) error {
	return s.mutate(ctx, func(tx CommerceCatalog) error { return tx.savePublicModel(ctx, actor, in, expected) })
}

// SetPublicModelEnabled is the list switch. It changes only whether customers
// can call the model, and it takes effect immediately. Other edits still use
// the advance-notice path.
func (s CommerceCatalog) SetPublicModelEnabled(ctx context.Context, actor, id string, enabled bool, expected uint64) error {
	if !s.platform(actor) {
		return ErrCatalogForbidden
	}
	return s.mutate(ctx, func(tx CommerceCatalog) error {
		m, e := tx.PublicModel(ctx, id)
		if e != nil {
			return e
		}
		if m.Enabled == enabled {
			return nil
		}
		if m.Version != expected {
			return ErrCatalogVersion
		}
		m.Enabled = enabled
		return tx.savePublicModel(context.WithValue(ctx, catalogApplyKey{}, true), actor, m, expected)
	})
}

func (s CommerceCatalog) EnabledRouteModelIDs(ctx context.Context) ([]string, error) {
	var ids []string
	e := s.DB.WithContext(ctx).Table("tokenhub_upstreams").Where("definition->>'enabled' = ?", "true").Distinct().Pluck("public_model_id", &ids).Error
	return ids, e
}
func (s CommerceCatalog) savePublicModel(ctx context.Context, actor string, in PublicModel, expected uint64) error {
	if !s.platform(actor) {
		return ErrCatalogForbidden
	}
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	if in.ID == "" || in.DisplayName == "" {
		return ErrInvalidInput
	}
	if expected > 0 {
		old, e := s.PublicModel(ctx, in.ID)
		if e != nil {
			return e
		}
		if in.Identity == "" {
			in.Identity = old.Identity
		}
		if old.Identity != in.Identity {
			return ErrInvalidInput
		}
		if !applying(ctx) {
			var listed int64
			if e = s.DB.WithContext(ctx).Table("tokenhub_model_listings").Where("public_model_id=? AND enabled", in.ID).Count(&listed).Error; e != nil {
				return e
			}
			if old.Enabled || listed > 0 {
				return ErrChangeNotice
			}
		}
	} else if in.Identity == "" {
		in.Identity = in.ID
	}
	in.Capabilities = publicCapabilityRecords(in.Capabilities)
	if in.Identity == "" || !validPublicCapabilities(in.Capabilities) {
		return ErrInvalidInput
	}
	if in.Enabled {
		upstreams, e := s.upstreams(ctx, in.ID)
		if e != nil {
			return e
		}
		for _, c := range in.Capabilities {
			found := false
			for _, u := range upstreams {
				if u.Enabled && u.Healthy && u.Identity == in.Identity && upstreamOffers(u.Capabilities, c.Operation) {
					found = true
					break
				}
			}
			if !found {
				return ErrPublicRoute
			}
		}
		if len(in.Capabilities) == 0 {
			return ErrPublicCapability
		}
	}
	in.Version = expected + 1
	return s.saveDefinition(ctx, "tokenhub_public_models", in.ID, in, expected, nil)
}
func (s CommerceCatalog) SaveUpstream(ctx context.Context, actor string, in UpstreamModel, expected uint64) error {
	return s.mutate(ctx, func(tx CommerceCatalog) error { return tx.saveUpstream(ctx, actor, in, expected) })
}
func (s CommerceCatalog) saveUpstream(ctx context.Context, actor string, in UpstreamModel, expected uint64) error {
	if !s.platform(actor) {
		return ErrCatalogForbidden
	}
	model, e := s.PublicModel(ctx, in.PublicModelID)
	if e != nil {
		return e
	}
	if in.ID == "" || in.ProviderID == "" || in.Provider == "" || in.Model == "" || in.CredentialID == "" || in.Identity != model.Identity {
		return ErrInvalidInput
	}
	if !in.Routing.valid() {
		return ErrInvalidInput
	}
	if in.Enabled {
		if len(in.Capabilities) == 0 {
			return modelruntime.ErrUnsupported
		}
		for _, c := range in.Capabilities {
			if !c.Verified() {
				return modelruntime.ErrUnsupported
			}
			if c.Operation == modelruntime.Video {
				for _, operation := range []modelruntime.Operation{modelruntime.VideoRetrieve, modelruntime.VideoDownload} {
					if _, e := modelruntime.ResolveTaskEndpoint(in.Endpoints[operation], "validation-task"); e != nil {
						return ErrInvalidInput
					}
				}
			}
		}
	}
	in.Version = expected + 1
	return s.saveDefinition(ctx, "tokenhub_upstreams", in.ID, in, expected, map[string]any{"public_model_id": in.PublicModelID, "provider_id": in.ProviderID})
}
func (s CommerceCatalog) saveDefinition(ctx context.Context, table, id string, value any, expected uint64, extra map[string]any) error {
	raw, e := json.Marshal(value)
	if e != nil {
		return e
	}
	fields := map[string]any{"id": id, "definition": string(raw), "version": expected + 1}
	for k, v := range extra {
		fields[k] = v
	}
	if expected == 0 {
		return s.DB.WithContext(ctx).Table(table).Create(fields).Error
	}
	delete(fields, "id")
	result := s.DB.WithContext(ctx).Table(table).Where("id=? AND version=?", id, expected).Updates(fields)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrCatalogVersion
	}
	return nil
}
func (s CommerceCatalog) PublicModel(ctx context.Context, id string) (PublicModel, error) {
	var row catalogJSONRow
	var result PublicModel
	e := s.DB.WithContext(ctx).Table("tokenhub_public_models").Where("id=?", id).Take(&row).Error
	if e == nil {
		e = json.Unmarshal(row.Definition, &result)
	}
	return result, e
}
func (s CommerceCatalog) upstreams(ctx context.Context, model string) ([]UpstreamModel, error) {
	var rows []catalogJSONRow
	e := s.DB.WithContext(ctx).Table("tokenhub_upstreams").Where("public_model_id=?", model).Order("id").Find(&rows).Error
	if e != nil {
		return nil, e
	}
	out := make([]UpstreamModel, 0, len(rows))
	for _, row := range rows {
		var u UpstreamModel
		if e = json.Unmarshal(row.Definition, &u); e != nil {
			return nil, e
		}
		out = append(out, u)
	}
	return out, nil
}

func (s CommerceCatalog) SaveCredential(ctx context.Context, actor, id, providerID, name, secret string, enabled bool, expected uint64) (CredentialView, error) {
	var view CredentialView
	err := s.mutate(ctx, func(tx CommerceCatalog) error {
		var e error
		view, e = tx.saveCredential(ctx, actor, id, providerID, name, secret, enabled, expected)
		return e
	})
	return view, err
}
func (s CommerceCatalog) saveCredential(ctx context.Context, actor, id, providerID, name, secret string, enabled bool, expected uint64) (CredentialView, error) {
	if !s.platform(actor) {
		return CredentialView{}, ErrCatalogForbidden
	}
	name = strings.TrimSpace(name)
	if id == "" || providerID == "" || name == "" || len([]rune(name)) > 80 || s.EncryptionKey == "" || (expected == 0 && secret == "") {
		return CredentialView{}, ErrInvalidInput
	}
	var cipher, fingerprint string
	var e error
	if secret == "" {
		var row struct{ Ciphertext, Fingerprint string }
		e = s.DB.WithContext(ctx).Table("tokenhub_provider_credentials").Select("ciphertext,fingerprint").Where("id=? AND provider_id=? AND version=?", id, providerID, expected).Take(&row).Error
		if e != nil {
			return CredentialView{}, ErrCatalogVersion
		}
		cipher, fingerprint = row.Ciphertext, row.Fingerprint
	} else {
		cipher, e = platformcrypto.Seal(s.EncryptionKey, secret)
		if e != nil {
			return CredentialView{}, e
		}
		fingerprint = platformcrypto.HashToken(secret)[:12]
	}
	fields := map[string]any{"id": id, "provider_id": providerID, "name": name, "ciphertext": cipher, "fingerprint": fingerprint, "enabled": enabled, "version": expected + 1}
	if expected == 0 {
		e = s.DB.WithContext(ctx).Table("tokenhub_provider_credentials").Create(fields).Error
	} else {
		delete(fields, "id")
		r := s.DB.WithContext(ctx).Table("tokenhub_provider_credentials").Where("id=? AND provider_id=? AND version=?", id, providerID, expected).Updates(fields)
		e = r.Error
		if e == nil && r.RowsAffected != 1 {
			e = ErrCatalogVersion
		}
	}
	return CredentialView{ID: id, Name: name, ProviderID: providerID, Fingerprint: fingerprint, Enabled: enabled, Version: expected + 1}, e
}

// ResolveCredential satisfies modelruntime.CredentialSource. It pins the secret
// to this upstream's provider; one provider's key can never leak to another.
func (s CommerceCatalog) ResolveCredential(ctx context.Context, upstreamID, credentialID string) (modelruntime.Credential, error) {
	var row struct {
		ID, ProviderID, Ciphertext string
		Enabled                    bool
	}
	e := s.DB.WithContext(ctx).Table("tokenhub_provider_credentials AS c").Select("c.id,c.provider_id,c.ciphertext,c.enabled").Joins("JOIN tokenhub_upstreams u ON u.provider_id=c.provider_id AND u.id=? AND u.definition->>'credential_id'=c.id AND (u.definition->>'enabled')::boolean=true", upstreamID).Where("c.id=? AND c.enabled=true", credentialID).Take(&row).Error
	if e != nil {
		return modelruntime.Credential{}, e
	}
	plain, e := platformcrypto.Open(s.EncryptionKey, row.Ciphertext)
	if e != nil {
		return modelruntime.Credential{}, errors.New("platform credential cannot be decrypted")
	}
	return modelruntime.Credential{ID: row.ID, Secret: plain}, nil
}

func (s CommerceCatalog) SetAuthorization(ctx context.Context, actor, buyer, model string, enabled bool, expected uint64) error {
	return s.mutate(ctx, func(tx CommerceCatalog) error {
		return tx.setAuthorization(ctx, actor, buyer, model, enabled, expected)
	})
}
func (s CommerceCatalog) setAuthorization(ctx context.Context, actor, buyer, model string, enabled bool, expected uint64) error {
	if expected > 0 && !applying(ctx) {
		return ErrChangeNotice
	}
	if s.Operators == nil {
		return ErrCatalogForbidden
	}
	parent, e := s.Operators.CatalogOperator(ctx, actor)
	if e != nil {
		return e
	}
	child, e := s.Operators.CatalogOperator(ctx, buyer)
	if e != nil {
		return e
	}
	if child.ParentID != actor || (parent.Kind != "platform" && parent.Kind != "oem") || (parent.Kind == "oem" && (child.Kind != "channel" || child.BrandID != parent.BrandID)) || (parent.Kind == "platform" && !s.platform(actor)) {
		return ErrCatalogForbidden
	}
	if enabled {
		if e = s.authorized(ctx, actor, model); e != nil {
			return e
		}
	}
	return s.setFlag(ctx, "tokenhub_model_authorizations", map[string]any{"seller_id": actor, "buyer_id": buyer, "public_model_id": model}, enabled, expected)
}
func (s CommerceCatalog) SetListing(ctx context.Context, actor, model string, enabled bool, expected uint64) error {
	return s.mutate(ctx, func(tx CommerceCatalog) error { return tx.setListing(ctx, actor, model, enabled, expected) })
}
func (s CommerceCatalog) setListing(ctx context.Context, actor, model string, enabled bool, expected uint64) error {
	if expected > 0 {
		var n int64
		if e := s.DB.WithContext(ctx).Table("tokenhub_model_listings").Where("operator_id=? AND public_model_id=? AND version=?", actor, model, expected).Count(&n).Error; e != nil {
			return e
		}
		if n != 1 {
			return ErrCatalogVersion
		}
	}
	if !applying(ctx) {
		var count int64
		if e := s.DB.WithContext(ctx).Table("tokenhub_model_listings").Where("operator_id=?", actor).Count(&count).Error; e != nil {
			return e
		}
		if expected > 0 || count > 0 {
			return ErrChangeNotice
		}
	}
	if actor == "" {
		return ErrCatalogForbidden
	}
	if enabled {
		if e := s.authorized(ctx, actor, model); e != nil {
			return e
		}
	}
	return s.setFlag(ctx, "tokenhub_model_listings", map[string]any{"operator_id": actor, "public_model_id": model}, enabled, expected)
}
func (s CommerceCatalog) setFlag(ctx context.Context, table string, keys map[string]any, enabled bool, expected uint64) error {
	if expected == 0 {
		fields := map[string]any{}
		for k, v := range keys {
			fields[k] = v
		}
		fields["enabled"] = enabled
		fields["version"] = uint64(1)
		return s.DB.WithContext(ctx).Table(table).Create(fields).Error
	}
	r := s.DB.WithContext(ctx).Table(table).Where(keys).Where("version=?", expected).Updates(map[string]any{"enabled": enabled, "version": expected + 1})
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return ErrCatalogVersion
	}
	return nil
}
func (s CommerceCatalog) authorized(ctx context.Context, operator, model string) error {
	return s.requireAuthorization(ctx, operator, model, true)
}
func (s CommerceCatalog) requireAuthorization(ctx context.Context, operator, model string, requireEnabled bool) error {
	m, e := s.PublicModel(ctx, model)
	if e != nil {
		return e
	}
	if requireEnabled && !m.Enabled {
		return ErrModelNotVisible
	}
	seen := map[string]bool{}
	for operator != s.PlatformOperatorID {
		if operator == "" || seen[operator] || len(seen) >= 3 || s.Operators == nil {
			return ErrCatalogForbidden
		}
		seen[operator] = true
		o, e := s.Operators.CatalogOperator(ctx, operator)
		if e != nil {
			return e
		}
		var count int64
		e = s.DB.WithContext(ctx).Table("tokenhub_model_authorizations").Where("seller_id=? AND buyer_id=? AND public_model_id=? AND enabled=true", o.ParentID, operator, model).Count(&count).Error
		if e != nil {
			return e
		}
		if count != 1 {
			return ErrModelNotVisible
		}
		operator = o.ParentID
	}
	return nil
}

// Routes checks brand, ancestor authorizations, self listing, public capability,
// upstream identity, all requested parameters and a finite charging upper bound.
// It does not inspect parent listings: authorizing supply is separate from retail.
func (s CommerceCatalog) Routes(ctx context.Context, brand, operator string, r modelruntime.Request) ([]UpstreamModel, error) {
	if s.Operators == nil {
		return nil, ErrCatalogForbidden
	}
	o, e := s.Operators.CatalogOperator(ctx, operator)
	if e != nil {
		return nil, e
	}
	if o.BrandID != brand {
		return nil, ErrCatalogForbidden
	}
	if e = s.authorized(ctx, operator, r.PublicModel); e != nil {
		return nil, e
	}
	var count int64
	e = s.DB.WithContext(ctx).Table("tokenhub_model_listings").Where("operator_id=? AND public_model_id=? AND enabled=true", operator, r.PublicModel).Count(&count).Error
	if e != nil {
		return nil, e
	}
	if count != 1 {
		return nil, ErrModelNotVisible
	}
	m, e := s.PublicModel(ctx, r.PublicModel)
	if e != nil {
		return nil, e
	}
	if !operationOffered(m.Capabilities, r) {
		return nil, modelruntime.ErrUnsupported
	}
	upstreams, e := s.upstreams(ctx, r.PublicModel)
	if e != nil {
		return nil, e
	}
	health, now, e := s.routeHealth(ctx, r.PublicModel)
	if e != nil {
		return nil, e
	}
	out := make([]UpstreamModel, 0)
	for _, u := range upstreams {
		h := health[u.ID]
		if u.Enabled && u.Healthy && u.Identity == m.Identity && requestSupported(u.Capabilities, r) && (h.OpenUntil == nil || !h.OpenUntil.After(now)) {
			out = append(out, u)
		}
	}
	if len(out) == 0 {
		return nil, modelruntime.ErrUnsupported
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority < out[j].Priority
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// SelectCapability returns the exact verified capability admitted for a request.
func SelectCapability(u UpstreamModel, r modelruntime.Request) (modelruntime.Capability, error) {
	for _, c := range u.Capabilities {
		if requestSupported([]modelruntime.Capability{c}, r) {
			return c, nil
		}
	}
	return modelruntime.Capability{}, modelruntime.ErrUnsupported
}
func requestSupported(capabilities []modelruntime.Capability, r modelruntime.Request) bool {
	for _, c := range capabilities {
		a := modelruntime.Attempt{ID: "validate", RequestID: r.ID, UpstreamID: "validate", CredentialID: "validate", PublicModel: r.PublicModel, UpstreamModel: "validate", Deadline: time.Now().Add(time.Minute), Capability: c}
		if modelruntime.ValidateRequest(a, r) == nil {
			return true
		}
	}
	return false
}
func (s CommerceCatalog) PriceVersions(ctx context.Context, scope PriceScope, model string) ([]PriceVersion, error) {
	var rows []catalogJSONRow
	e := s.DB.WithContext(ctx).Table("tokenhub_model_price_versions").Select("id,definition").Where("scope_key=? AND public_model_id IN ?", scope.Key(), []string{model, "*"}).Order("effective_at DESC").Find(&rows).Error
	if e != nil {
		return nil, e
	}
	out := make([]PriceVersion, 0, len(rows))
	for _, row := range rows {
		var v PriceVersion
		if e = json.Unmarshal(row.Definition, &v); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, nil
}

type ModelQuote struct {
	CatalogVersion uint64                  `json:"catalog_version"`
	ModelID        string                  `json:"model_id"`
	AcceptedAt     time.Time               `json:"accepted_at"`
	Retail         ResolvedPrice           `json:"retail"`
	Supply         []ResolvedPrice         `json:"supply"` // serving operator outward to platform
	Supplier       []SupplierPriceSnapshot `json:"supplier"`
	Bounds         map[string]string       `json:"bounds"`
	Candidates     []UpstreamModel         `json:"-"`
}
type SupplierPriceSnapshot struct {
	UpstreamID string        `json:"upstream_id"`
	Price      ResolvedPrice `json:"price"`
}

// Quote resolves all price books and takes a component-wise maximum across
// every eligible fallback candidate. Invoke it on the same caller transaction
// that locks the catalog/coverage version and creates reservations.
func (s CommerceCatalog) Quote(ctx context.Context, brand, operator string, r modelruntime.Request, at time.Time) (ModelQuote, error) {
	revision, e := s.LockRevision(ctx)
	if e != nil {
		return ModelQuote{}, e
	}
	upstreams, e := s.Routes(ctx, brand, operator, r)
	if e != nil {
		return ModelQuote{}, e
	}
	resolver := PriceResolver{Source: s}
	retail, e := resolver.Resolve(ctx, PriceScope{Kind: "retail", SellerID: operator}, r.PublicModel, at)
	if e != nil {
		return ModelQuote{}, e
	}
	q := ModelQuote{CatalogVersion: revision, ModelID: r.PublicModel, AcceptedAt: at, Retail: retail, Bounds: map[string]string{}, Candidates: upstreams}
	for _, u := range upstreams {
		cap, e := SelectCapability(u, r)
		if e != nil {
			return ModelQuote{}, e
		}
		for _, rule := range cap.UsageRules {
			found := false
			for _, rate := range retail.Components {
				if rate.Key == rule.Component && rate.Unit == rule.Unit {
					found = true
					break
				}
			}
			if !found {
				return ModelQuote{}, fmt.Errorf("unpriced metering component %s", rule.Component)
			}
		}
		for key, raw := range cap.BillableBounds {
			n, e := PriceDecimal(raw)
			if e != nil {
				return ModelQuote{}, e
			}
			prev, e := PriceDecimal(q.Bounds[key])
			if e != nil || n.Cmp(prev) > 0 {
				q.Bounds[key] = raw
			}
		}
		// Supplier USD cost belongs to the supplier model. A route saved before
		// that page existed still resolves the price book keyed by the upstream.
		var cost ResolvedPrice
		if u.SupplierModelID != "" {
			supplierModel, modelErr := s.SupplierModel(ctx, u.SupplierModelID)
			if modelErr != nil {
				return ModelQuote{}, modelErr
			}
			if !supplierModel.Enabled {
				return ModelQuote{}, ErrPriceUnavailable
			}
			cost, e = supplierModelPrice(supplierModel, u.ID, r.PublicModel, at)
		} else {
			cost, e = resolver.Resolve(ctx, PriceScope{Kind: "supplier", SellerID: u.ProviderID, BuyerID: u.ID}, r.PublicModel, at)
		}
		if e != nil {
			return ModelQuote{}, e
		}
		if _, e = cost.MaximumCharge(cap.BillableBounds); e != nil {
			return ModelQuote{}, e
		}
		if _, e = CoverageFactor(retail, cost); e != nil {
			return ModelQuote{}, e
		}
		q.Supplier = append(q.Supplier, SupplierPriceSnapshot{UpstreamID: u.ID, Price: cost})
	}
	if _, e = retail.MaximumCharge(q.Bounds); e != nil {
		return ModelQuote{}, e
	}
	for current := operator; current != s.PlatformOperatorID; {
		o, e := s.Operators.CatalogOperator(ctx, current)
		if e != nil {
			return ModelQuote{}, e
		}
		price, e := resolver.Resolve(ctx, PriceScope{Kind: "supply", SellerID: o.ParentID, BuyerID: current}, r.PublicModel, at)
		if e != nil {
			return ModelQuote{}, e
		}
		if _, e = price.MaximumCharge(q.Bounds); e != nil {
			return ModelQuote{}, e
		}
		if _, e = CoverageFactor(retail, price); e != nil {
			return ModelQuote{}, e
		}
		q.Supply = append(q.Supply, price)
		current = o.ParentID
		if len(q.Supply) > 3 {
			return ModelQuote{}, ErrCatalogForbidden
		}
	}
	if e = s.rankRoutes(ctx, &q); e != nil {
		return ModelQuote{}, e
	}
	return q, nil
}

// PublishPrice requires a caller's coverage computation in the same transaction.
// The validator must lock/check the authoritative coverage version; simply
// returning nil without checking coverage is not an acceptable integration.
func (s CommerceCatalog) PublishPrice(ctx context.Context, actor string, v PriceVersion, validateCoverage func(context.Context, *gorm.DB, PriceVersion) error) error {
	return s.mutate(ctx, func(tx CommerceCatalog) error { return tx.publishPrice(ctx, actor, v, validateCoverage) })
}
func (s CommerceCatalog) publishPrice(ctx context.Context, actor string, v PriceVersion, validateCoverage func(context.Context, *gorm.DB, PriceVersion) error) error {
	if ((v.Scope.Kind != "supplier" && actor != v.Scope.SellerID) || (v.Scope.Kind == "supplier" && !s.platform(actor))) || validateCoverage == nil {
		return ErrCatalogForbidden
	}
	if e := ValidatePriceVersion(v); e != nil {
		return e
	}
	if v.ModelID != "*" {
		if _, e := s.PublicModel(ctx, v.ModelID); e != nil {
			return e
		}
	}
	if v.Scope.Kind == "supplier" {
		var count int64
		if e := s.DB.WithContext(ctx).Table("tokenhub_upstreams").Where("id=? AND provider_id=? AND public_model_id=?", v.Scope.BuyerID, v.Scope.SellerID, v.ModelID).Count(&count).Error; e != nil {
			return e
		}
		if count != 1 {
			return ErrInvalidInput
		}
	}
	if !v.Published {
		return ErrChangeNotice
	}
	if v.Scope.Kind != "supplier" {
		var existing int64
		if e := s.DB.WithContext(ctx).Table("tokenhub_model_price_versions").Where("scope_key=?", v.Scope.Key()).Count(&existing).Error; e != nil {
			return e
		}
		if existing > 0 {
			if e := s.requirePriceNotice(ctx, v); e != nil {
				return e
			}
		}
	}

	if v.Scope.Kind == "supply" {
		if s.Operators == nil {
			return ErrCatalogForbidden
		}
		buyer, e := s.Operators.CatalogOperator(ctx, v.Scope.BuyerID)
		if e != nil {
			return e
		}
		if buyer.ParentID != actor {
			return ErrCatalogForbidden
		}
	}
	if v.Rule.Mode == "follow" {
		if s.Operators == nil {
			return ErrCatalogForbidden
		}
		seller, e := s.Operators.CatalogOperator(ctx, actor)
		if e != nil {
			return e
		}
		if v.Rule.Follow.Kind != "supply" || v.Rule.Follow.SellerID != seller.ParentID || v.Rule.Follow.BuyerID != actor {
			return ErrCatalogForbidden
		}
	}
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", v.Scope.Key()+":"+v.ModelID).Error; e != nil {
			return e
		}
		if v.Layer == "promotion" {
			var count int64
			e := tx.Table("tokenhub_model_price_versions").Where("scope_key=? AND public_model_id=? AND layer='promotion' AND effective_at<? AND (ends_at IS NULL OR ends_at>?)", v.Scope.Key(), v.ModelID, *v.EndsAt, v.EffectiveAt).Count(&count).Error
			if e != nil {
				return e
			}
			if count > 0 {
				return errors.New("overlapping model promotions")
			}
		}
		raw, e := json.Marshal(v)
		if e != nil {
			return e
		}
		if e = tx.Table("tokenhub_model_price_versions").Create(map[string]any{"id": v.ID, "scope_key": v.Scope.Key(), "public_model_id": v.ModelID, "layer": v.Layer, "effective_at": v.EffectiveAt, "ends_at": v.EndsAt, "definition": string(raw)}).Error; e != nil {
			return e
		}
		if e = validateCoverage(ctx, tx, v); e != nil {
			return fmt.Errorf("price coverage validation: %w", e)
		}
		return nil
	})
}
