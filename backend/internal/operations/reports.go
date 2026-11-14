package operations

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/platform/objectstore"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Reconciliation struct {
	ID           string    `json:"id"`
	OperatorID   string    `json:"operator_id"`
	CreatedAt    time.Time `json:"created_at"`
	CreatedBy    string    `json:"created_by"`
	FindingCount int64     `json:"finding_count"`
}

func (Reconciliation) TableName() string { return "operations_reconciliations" }

type Discrepancy struct {
	ID                  string         `json:"id"`
	RunID               string         `json:"run_id"`
	OperatorID          string         `json:"operator_id"`
	Domain              string         `json:"domain"`
	Kind                string         `json:"kind"`
	Reference           string         `json:"reference"`
	Unit                string         `json:"unit"`
	Expected            ledger.Decimal `json:"expected"`
	Actual              ledger.Decimal `json:"actual"`
	State               string         `json:"state"`
	AdjustmentReference *string        `json:"adjustment_reference,omitempty"`
	ResolvedBy          *string        `json:"resolved_by,omitempty"`
	ResolvedAt          *time.Time     `json:"resolved_at,omitempty"`
	ResolutionReason    string         `json:"resolution_reason,omitempty"`
}

func (Discrepancy) TableName() string { return "operations_discrepancies" }
func (s *Service) RunReconciliation(ctx context.Context, actor, operator string) (Reconciliation, error) {
	out := Reconciliation{ID: uuid.NewString(), OperatorID: operator, CreatedBy: actor, CreatedAt: time.Now().UTC()}
	if s.Source == nil {
		return out, ErrNotReady
	}
	run := func(t *gorm.DB) error {
		if e := s.require(ctx, t, actor, operator, "reports.view"); e != nil {
			return e
		}
		findings, e := s.Source.Reconcile(ctx, t, operator)
		if e != nil {
			return e
		}
		out.FindingCount = int64(len(findings))
		if e = t.Create(&out).Error; e != nil {
			return e
		}
		for _, f := range findings {
			d := Discrepancy{ID: uuid.NewString(), RunID: out.ID, OperatorID: operator, Domain: f.Domain, Kind: f.Kind, Reference: f.Reference, Unit: f.Unit, Expected: f.Expected, Actual: f.Actual, State: "open"}
			if e = t.Clauses(clause.OnConflict{
				Columns:     []clause.Column{{Name: "operator_id"}, {Name: "domain"}, {Name: "kind"}, {Name: "reference"}, {Name: "unit"}},
				TargetWhere: clause.Where{Exprs: []clause.Expression{clause.Expr{SQL: "state='open'"}}},
				DoUpdates:   clause.AssignmentColumns([]string{"run_id", "expected", "actual"}),
			}).Create(&d).Error; e != nil {
				return e
			}
		}
		return nil
	}
	var err error
	for range 3 {
		err = s.DB.WithContext(ctx).Transaction(run, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
		var pg *pgconn.PgError
		if !errors.As(err, &pg) || pg.Code != "40001" {
			break
		}
	}
	return out, err
}

type discrepancyCursor struct {
	State, ID string
}

func (s *Service) ListDiscrepancies(ctx context.Context, actor, operator, state, cursor string, limit int) ([]Discrepancy, string, error) {
	if limit < 1 || limit > 100 || (state != "" && state != "open" && state != "resolved") {
		return nil, "", ErrInvalid
	}
	var after discrepancyCursor
	if cursor != "" {
		data, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil || json.Unmarshal(data, &after) != nil || !validID(after.ID) ||
			(after.State != "open" && after.State != "resolved") || (state != "" && state != after.State) {
			return nil, "", ErrInvalid
		}
	}
	out := []Discrepancy{}
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if e := s.require(ctx, t, actor, operator, "reports.view"); e != nil {
			return e
		}
		q := t.Where("operator_id=?", operator)
		if state != "" {
			q = q.Where("state=?", state)
		}
		if cursor != "" {
			q = q.Where("(state,id)>(?,?)", after.State, after.ID)
		}
		return q.Order("state,id").Limit(limit + 1).Find(&out).Error
	})
	next := ""
	if e == nil && len(out) > limit {
		out = out[:limit]
		last := out[limit-1]
		data, _ := json.Marshal(discrepancyCursor{last.State, last.ID})
		next = base64.RawURLEncoding.EncodeToString(data)
	}
	return out, next, e
}

// Resolve requires both a real adjustment and a fresh report proving that this
// exact discrepancy disappeared. A reference alone cannot hide an open mismatch.
func (s *Service) ResolveDiscrepancy(ctx context.Context, actor, operator, id, adjustment, reason string) error {
	if s.Source == nil {
		return ErrNotReady
	}
	if reason == "" || !validID(adjustment) {
		return ErrInvalid
	}
	return s.within(ctx, nil, func(t *gorm.DB) error {
		if e := s.require(ctx, t, actor, operator, "operations.manage"); e != nil {
			return e
		}
		var d Discrepancy
		if e := t.Clauses(clause.Locking{Strength: "UPDATE"}).First(&d, "id=? AND operator_id=?", id, operator).Error; e != nil {
			return e
		}
		if d.State == "resolved" {
			if d.AdjustmentReference != nil && *d.AdjustmentReference == adjustment {
				return nil
			}
			return ErrConflict
		}
		ok, e := s.Source.HasAdjustment(ctx, t, d, adjustment)
		if e != nil {
			return e
		}
		if !ok {
			return ErrInvalid
		}
		findings, e := s.Source.Reconcile(ctx, t, operator)
		if e != nil {
			return e
		}
		for _, f := range findings {
			if f.Domain == d.Domain && f.Kind == d.Kind && f.Reference == d.Reference && f.Unit == d.Unit {
				return ErrConflict
			}
		}
		return t.Model(&d).Updates(map[string]any{"state": "resolved", "adjustment_reference": adjustment, "resolved_by": actor, "resolved_at": time.Now().UTC(), "resolution_reason": reason}).Error
	})
}

type Export struct {
	ID          string     `json:"id"`
	OperatorID  string     `json:"operator_id"`
	RequestedBy string     `json:"requested_by"`
	Kind        string     `json:"kind"`
	FromAt      time.Time  `json:"from_at"`
	ToAt        time.Time  `json:"to_at"`
	State       string     `json:"state"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   time.Time  `json:"expires_at"`
	LeaseUntil  *time.Time `json:"-"`
	ObjectKey   string     `json:"-"`
	AttemptID   *string    `json:"-"`
	SHA256      string     `json:"sha256,omitempty"`
	RowCount    int        `json:"row_count"`
	ErrorCode   string     `json:"error_code,omitempty"`
}

func (Export) TableName() string { return "operations_exports" }
func (s *Service) RequestExport(ctx context.Context, actor, operator string, q ExportQuery) (Export, error) {
	out := Export{}
	if q.From.IsZero() || !q.To.After(q.From) || q.To.Sub(q.From) > 366*24*time.Hour || !exportKind(q.Kind) {
		return out, ErrInvalid
	}
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if e := s.require(ctx, t, actor, operator, "reports.export"); e != nil {
			return e
		}
		if q.Kind == "financial" {
			if e := s.require(ctx, t, actor, operator, "reports.view"); e != nil {
				return e
			}
		}
		now := time.Now().UTC()
		out = Export{ID: uuid.NewString(), OperatorID: operator, RequestedBy: actor, Kind: q.Kind, FromAt: q.From.UTC(), ToAt: q.To.UTC(), State: "pending", CreatedAt: now, ExpiresAt: now.Add(24 * time.Hour)}
		return t.Create(&out).Error
	})
	return out, e
}
func (s *Service) ListExports(ctx context.Context, actor, operator string) ([]Export, error) {
	out := []Export{}
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if e := s.require(ctx, t, actor, operator, "reports.export"); e != nil {
			return e
		}
		return t.Where("operator_id=? AND requested_by=?", operator, actor).Order("created_at DESC").Limit(100).Find(&out).Error
	})
	return out, e
}
func (s *Service) RunExports(ctx context.Context, limit int) ([]Export, error) {
	if s.Source == nil || s.ExportStore == nil {
		return nil, ErrNotReady
	}
	if limit < 1 || limit > 20 {
		limit = 5
	}
	jobs := []Export{}
	e := s.DB.WithContext(ctx).Transaction(func(t *gorm.DB) error {
		var now time.Time
		if err := t.Raw("SELECT clock_timestamp()").Row().Scan(&now); err != nil {
			return err
		}
		if e := t.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Where("expires_at>? AND (state='pending' OR (state='processing' AND lease_until<?))", now, now).Order("created_at").Limit(limit).Find(&jobs).Error; e != nil {
			return e
		}
		for i := range jobs {
			j := &jobs[i]
			id := uuid.NewString()
			j.AttemptID = &id
			if e := t.Model(&Export{}).Where("id=?", j.ID).Updates(map[string]any{"state": "processing", "lease_until": now.Add(time.Minute), "attempt_id": id}).Error; e != nil {
				return e
			}
		}
		return nil
	})
	if e != nil {
		return nil, e
	}
	completed := make([]Export, 0, len(jobs))
	for i := range jobs {
		j := &jobs[i]
		j.State = "ready"
		j.ErrorCode = ""
		var ciphertext string
		e = s.DB.WithContext(ctx).Transaction(func(t *gorm.DB) error {
			if e := s.require(ctx, t, j.RequestedBy, j.OperatorID, "reports.export"); e != nil {
				return e
			}
			if j.Kind == "financial" {
				if e := s.require(ctx, t, j.RequestedBy, j.OperatorID, "reports.view"); e != nil {
					return e
				}
			}
			rows, e := s.Source.Export(ctx, t, j.OperatorID, ExportQuery{Kind: j.Kind, From: j.FromAt, To: j.ToAt})
			if e != nil {
				return e
			}
			if len(rows) > 100000 {
				return ErrTooLarge
			}
			b, e := buildExportCSV(ctx, t, j.OperatorID, j.Kind, rows)
			if e != nil {
				return e
			}
			if len(b) > 64<<20 {
				return ErrTooLarge
			}
			digest := sha256.Sum256(b)
			j.SHA256 = hex.EncodeToString(digest[:])
			j.RowCount = len(rows)
			ciphertext, e = s.seal(b)
			return e
		}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
		if e == nil {
			j.ObjectKey = "exports/" + j.ID + "/" + *j.AttemptID
			e = s.ExportStore.Put(ctx, j.ObjectKey, []byte(ciphertext), "application/octet-stream", j.ExpiresAt)
			if errors.Is(e, objectstore.ErrSize) {
				e = ErrTooLarge
			}
		}
		if e != nil {
			j.State = "failed"
			j.ErrorCode = "source_unavailable"
			if errors.Is(e, ErrTooLarge) {
				j.ErrorCode = "range_too_large"
			}
			j.ObjectKey, j.SHA256, j.RowCount = "", "", 0
		}
		result := s.DB.WithContext(ctx).Model(&Export{}).
			Where("id=? AND state='processing' AND attempt_id=? AND lease_until>clock_timestamp() AND expires_at>clock_timestamp()", j.ID, *j.AttemptID).
			Updates(map[string]any{"state": j.State, "object_key": j.ObjectKey, "sha256": j.SHA256, "row_count": j.RowCount, "error_code": j.ErrorCode, "lease_until": nil, "attempt_id": nil})
		if result.Error != nil {
			return completed, result.Error
		}
		if result.RowsAffected == 0 {
			if j.ObjectKey != "" {
				_ = s.ExportStore.Delete(ctx, j.ObjectKey)
			}
			continue
		}
		completed = append(completed, *j)
	}
	return completed, nil
}
func (s *Service) DownloadExport(ctx context.Context, actor, operator, id string) ([]byte, error) {
	var out []byte
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if e := s.require(ctx, t, actor, operator, "reports.export"); e != nil {
			return e
		}
		var row Export
		if e := t.First(&row, "id=? AND operator_id=? AND requested_by=?", id, operator, actor).Error; e != nil {
			return e
		}
		if row.State != "ready" || !time.Now().UTC().Before(row.ExpiresAt) {
			return ErrExpired
		}
		if row.Kind == "financial" {
			if e := s.require(ctx, t, actor, operator, "reports.view"); e != nil {
				return e
			}
		}
		if s.ExportStore == nil {
			return ErrNotReady
		}
		ciphertext, e := s.ExportStore.Get(ctx, row.ObjectKey)
		if e != nil {
			return e
		}
		if e := s.open(string(ciphertext), &out); e != nil {
			return e
		}
		return t.Table("operations_export_access").Create(map[string]any{"id": uuid.NewString(), "export_id": id, "actor_id": actor, "at": time.Now().UTC()}).Error
	})
	return out, e
}
