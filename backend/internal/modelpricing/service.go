// Package modelpricing owns supplier cost estimates, the Bifrost price sheet,
// and simple manually recorded actual costs. It never sets customer ATG prices.
package modelpricing

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	bifrost "github.com/maximhq/bifrost/core"
	"gorm.io/gorm/clause"
	"io"
	"io/fs"
	"math"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/maximhq/bifrost/core/network"
	"github.com/maximhq/bifrost/framework/configstore"
	"github.com/maximhq/bifrost/framework/configstore/tables"
	"github.com/maximhq/bifrost/framework/modelcatalog/datasheet"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
)

//go:embed migrate/*.sql
var migrations embed.FS

func Migrations() (string, fs.FS) { f, _ := fs.Sub(migrations, "migrate"); return "model_pricing", f }

var ErrInput = errors.New("invalid model pricing input")
var ErrVersion = errors.New("model pricing version conflict")

type Settings struct {
	ID            string     `json:"id"`
	Version       uint64     `json:"version"`
	SheetVersion  string     `json:"sheet_version"`
	LastSuccessAt *time.Time `json:"last_success_at"`
	LastAttemptAt *time.Time `json:"last_attempt_at"`
	LastError     string     `json:"last_error"`
	SyncRequested bool       `json:"sync_requested"`
}

func (Settings) TableName() string { return "model_pricing_settings" }

type Row struct {
	ID         string          `json:"id"`
	Provider   string          `json:"provider"`
	Model      string          `json:"model"`
	Mode       string          `json:"mode"`
	Definition json.RawMessage `json:"-" gorm:"type:jsonb"`
}

func (Row) TableName() string { return "model_pricing_rows" }

type Snapshot struct {
	Version    string                     `json:"version"`
	AcceptedAt time.Time                  `json:"accepted_at"`
	Rows       []tables.TableModelPricing `json:"rows"`
}
type ActualCost struct {
	ID              string         `json:"id"`
	ProviderID      string         `json:"provider_id"`
	SupplierModelID string         `json:"supplier_model_id"`
	StartsAt        time.Time      `json:"starts_at"`
	EndsAt          time.Time      `json:"ends_at"`
	Currency        string         `json:"currency"`
	Amount          ledger.Decimal `json:"amount"`
	Note            string         `json:"note"`
	Version         uint64         `json:"version"`
	CreatedAt       time.Time      `json:"created_at"`
}

func (ActualCost) TableName() string { return "model_actual_costs" }

type Service struct {
	DB                *gorm.DB
	AllowTestLoopback bool
	// TestSourceURL is honored only by isolated integration test instances.
	TestSourceURL string
}

func (s *Service) Settings(ctx context.Context) (Settings, error) {
	var out Settings
	e := s.DB.WithContext(ctx).First(&out, "id='platform'").Error
	return out, e
}
func (s *Service) RequestSync(ctx context.Context) error {
	return s.DB.WithContext(ctx).Model(&Settings{}).Where("id='platform'").Update("sync_requested", true).Error
}

// priceRows exposes only the two ConfigStore methods used by Bifrost's
// datasheet loader. It keeps TokenHub's storage independent of gateway settings.
type priceRows struct {
	configstore.ConfigStore
	rows []tables.TableModelPricing
}

func (p *priceRows) GetModelPrices(context.Context) ([]tables.TableModelPricing, error) {
	return p.rows, nil
}
func (p *priceRows) UpsertModelPricesBatch(_ context.Context, rows []tables.TableModelPricing, _ ...*gorm.DB) error {
	p.rows = rows
	return nil
}

func (s *Service) RunSync(ctx context.Context) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var locked bool
		if e := tx.Raw("SELECT pg_try_advisory_xact_lock(hashtextextended('model-pricing-sync',0))").Scan(&locked).Error; e != nil {
			return e
		}
		if !locked {
			return nil
		}
		var cfg Settings
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&cfg, "id='platform'").Error; e != nil {
			return e
		}
		now := time.Now().UTC()
		if !cfg.SyncRequested && (cfg.LastSuccessAt != nil && now.Before(cfg.LastSuccessAt.Add(24*time.Hour))) {
			return nil
		}
		if !cfg.SyncRequested && cfg.LastAttemptAt != nil && now.Before(cfg.LastAttemptAt.Add(5*time.Minute)) {
			return nil
		}
		source := "https://getbifrost.ai/datasheet"
		if s.AllowTestLoopback && s.TestSourceURL != "" {
			source = s.TestSourceURL
		}
		payload, e := s.fetch(ctx, source)
		var rows []tables.TableModelPricing
		if e == nil {
			rows, e = parseSheet(ctx, payload)
		}
		if e != nil {
			// Keep the last good sheet. An unavailable feed must not erase prices or
			// make every process retry it once per request.
			return tx.Model(&Settings{}).Where("id='platform'").Updates(map[string]any{"last_attempt_at": now, "last_error": "价目同步失败，系统将自动重试。已有价目继续使用。", "sync_requested": false}).Error
		}
		digest := sha256.Sum256(payload)
		version := hex.EncodeToString(digest[:])
		if e = tx.Exec("DELETE FROM model_pricing_rows").Error; e != nil {
			return e
		}
		saved := make([]Row, 0, len(rows))
		for _, r := range rows {
			raw, _ := json.Marshal(r)
			saved = append(saved, Row{ID: r.Provider + "|" + r.Model + "|" + string(r.Mode), Provider: r.Provider, Model: r.Model, Mode: string(r.Mode), Definition: raw})
		}
		if len(saved) == 0 {
			return ErrInput
		}
		if e = tx.CreateInBatches(saved, 200).Error; e != nil {
			return e
		}
		return tx.Model(&Settings{}).Where("id='platform'").Updates(map[string]any{"sheet_version": version, "last_attempt_at": now, "last_success_at": now, "last_error": "", "sync_requested": false}).Error
	})
}
func parseSheet(ctx context.Context, raw []byte) ([]tables.TableModelPricing, error) {
	var entries map[string]json.RawMessage
	if json.Unmarshal(raw, &entries) != nil || len(entries) == 0 || len(entries) > 100000 {
		return nil, ErrInput
	}
	f, e := os.CreateTemp("", "tokenhub-pricing-*.json")
	if e != nil {
		return nil, e
	}
	defer os.Remove(f.Name())
	if _, e = f.Write(raw); e != nil {
		f.Close()
		return nil, e
	}
	if e = f.Close(); e != nil {
		return nil, e
	}
	rows := &priceRows{}
	store := datasheet.New(rows, bifrost.NewNoOpLogger(), datasheet.Config{URL: "file://" + f.Name()})
	if e = store.SyncFromURL(ctx); e != nil {
		return nil, e
	}
	filtered := []tables.TableModelPricing{}
	for _, row := range rows.rows {
		if row.Provider == "" || row.Model == "" || row.Mode == "" {
			continue
		}
		encoded, _ := json.Marshal(row)
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(encoded, &fields)
		priced := false
		for key, value := range fields {
			if strings.Contains(key, "cost") && string(value) != "null" {
				priced = true
				break
			}
		}
		if priced {
			filtered = append(filtered, row)
		}
	}
	if len(filtered) == 0 {
		return nil, ErrInput
	}
	return filtered, nil
}
func (s *Service) fetch(ctx context.Context, source string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	u, e := url.Parse(source)
	if e != nil || u.User != nil || u.Hostname() == "" || (u.Scheme != "https" && !(s.AllowTestLoopback && u.Scheme == "http")) {
		return nil, ErrInput
	}
	transport := &http.Transport{DialContext: func(c context.Context, networkName, address string) (net.Conn, error) {
		host, port, e := net.SplitHostPort(address)
		if e != nil {
			return nil, e
		}
		ips, e := net.DefaultResolver.LookupIP(c, "ip", host)
		if e != nil {
			return nil, e
		}
		for _, ip := range ips {
			if !network.IsPublicIP(ip) && !(s.AllowTestLoopback && ip.IsLoopback()) {
				return nil, ErrInput
			}
		}
		for _, ip := range ips {
			conn, e := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(c, networkName, net.JoinHostPort(ip.String(), port))
			if e == nil {
				return conn, nil
			}
		}
		return nil, ErrInput
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) > 3 || r.URL.Scheme != "https" {
			return ErrInput
		}
		return nil
	}}
	req, e := http.NewRequestWithContext(ctx, "GET", source, nil)
	if e != nil {
		return nil, e
	}
	resp, e := client.Do(req)
	if e != nil {
		return nil, e
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, ErrInput
	}
	raw, e := io.ReadAll(io.LimitReader(resp.Body, 32<<20+1))
	if e != nil || len(raw) > 32<<20 {
		return nil, ErrInput
	}
	return raw, nil
}
func (s *Service) Rows(ctx context.Context, search string) ([]Row, error) {
	var rows []Row
	q := s.DB.WithContext(ctx).Order("provider,model,mode").Limit(100)
	if search != "" {
		q = q.Where("id ILIKE ? OR model ILIKE ? OR provider ILIKE ?", search, "%"+search+"%", "%"+search+"%")
	}
	e := q.Find(&rows).Error
	return rows, e
}
func (s *Service) Snapshot(ctx context.Context, provider, model string, at time.Time) (json.RawMessage, error) {
	var raw json.RawMessage
	e := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []Row
		if e := tx.Where("provider=? AND model=?", provider, model).Find(&rows).Error; e != nil {
			return e
		}
		var cfg Settings
		if e := tx.First(&cfg, "id='platform'").Error; e != nil {
			return e
		}
		out := Snapshot{Version: cfg.SheetVersion, AcceptedAt: at}
		for _, r := range rows {
			var row tables.TableModelPricing
			if json.Unmarshal(r.Definition, &row) != nil {
				return ErrInput
			}
			out.Rows = append(out.Rows, row)
		}
		var e error
		raw, e = json.Marshal(out)
		return e
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return raw, e
}
func validCurrency(currency string) bool {
	if len(currency) < 2 || len(currency) > 12 {
		return false
	}
	for _, c := range currency {
		if c < 'A' || c > 'Z' {
			return false
		}
	}
	return true
}

func (s *Service) SaveActual(ctx context.Context, in ActualCost, expected uint64) error {
	in.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
	in.Note = strings.TrimSpace(in.Note)
	if in.ProviderID == "" || in.StartsAt.IsZero() || !in.EndsAt.After(in.StartsAt) || !in.Amount.Valid() || in.Amount.Sign() < 0 || !validCurrency(in.Currency) || len(in.Note) > 2000 {
		return ErrInput
	}
	var count int64
	if e := s.DB.WithContext(ctx).Table("tokenhub_provider_credentials").Where("provider_id=?", in.ProviderID).Count(&count).Error; e != nil {
		return e
	}
	if count == 0 {
		return ErrInput
	}
	if in.SupplierModelID != "" {
		if e := s.DB.WithContext(ctx).Table("tokenhub_supplier_models").Where("id=? AND provider_id=?", in.SupplierModelID, in.ProviderID).Count(&count).Error; e != nil {
			return e
		}
		if count == 0 {
			return ErrInput
		}
	}
	in.Version = expected + 1
	if expected == 0 {
		if in.ID == "" {
			in.ID = uuid.NewString()
		}
		// Registration time belongs to the database. The statement's service
		// period is supplied separately and must not backdate financial reports.
		if e := s.DB.WithContext(ctx).Raw("SELECT clock_timestamp()").Scan(&in.CreatedAt).Error; e != nil {
			return e
		}
		return s.DB.WithContext(ctx).Create(&in).Error
	}
	r := s.DB.WithContext(ctx).Model(&ActualCost{}).Where("id=? AND version=?", in.ID, expected).Updates(map[string]any{"provider_id": in.ProviderID, "supplier_model_id": in.SupplierModelID, "starts_at": in.StartsAt, "ends_at": in.EndsAt, "currency": in.Currency, "amount": in.Amount, "note": strings.TrimSpace(in.Note), "version": in.Version})
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return ErrVersion
	}
	return nil
}
func (s *Service) Actuals(ctx context.Context) ([]ActualCost, error) {
	var out []ActualCost
	e := s.DB.WithContext(ctx).Order("created_at DESC,id DESC").Limit(100).Find(&out).Error
	return out, e
}

func rowStore(ctx context.Context, snapshot Snapshot) (*datasheet.Store, error) {
	p := &priceRows{rows: snapshot.Rows}
	store := datasheet.New(p, bifrost.NewNoOpLogger(), datasheet.Config{})
	if e := store.LoadFromDB(ctx); e != nil {
		return nil, e
	}
	return store, nil
}
func nonnegative(value float64) *string {
	if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return nil
	}
	raw := strconv.FormatFloat(value, 'f', -1, 64)
	if len(raw) > 100 {
		return nil
	}
	if n, ok := new(big.Rat).SetString(raw); !ok || n.Sign() < 0 {
		return nil
	}
	return &raw
}
