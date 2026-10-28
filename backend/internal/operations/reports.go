package operations

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
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
	e := s.DB.WithContext(ctx).Transaction(func(t *gorm.DB) error {
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
			if e = t.Create(&d).Error; e != nil {
				return e
			}
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	return out, e
}
func (s *Service) ListDiscrepancies(ctx context.Context, actor, operator string, limit int) ([]Discrepancy, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	out := []Discrepancy{}
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if e := s.require(ctx, t, actor, operator, "reports.view"); e != nil {
			return e
		}
		return t.Where("operator_id=?", operator).Order("state,id").Limit(limit).Find(&out).Error
	})
	return out, e
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
		ok, e := s.Source.HasAdjustment(ctx, t, operator, adjustment)
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
			if f.Domain == d.Domain && f.Kind == d.Kind && f.Reference == d.Reference {
				return ErrConflict
			}
		}
		return t.Model(&d).Updates(map[string]any{"state": "resolved", "adjustment_reference": adjustment, "resolved_by": actor, "resolved_at": time.Now().UTC(), "resolution_reason": reason}).Error
	})
}

type Export struct {
	ID                string     `json:"id"`
	OperatorID        string     `json:"operator_id"`
	RequestedBy       string     `json:"requested_by"`
	Kind              string     `json:"kind"`
	FromAt            time.Time  `json:"from_at"`
	ToAt              time.Time  `json:"to_at"`
	State             string     `json:"state"`
	CreatedAt         time.Time  `json:"created_at"`
	ExpiresAt         time.Time  `json:"expires_at"`
	LeaseUntil        *time.Time `json:"-"`
	ContentCiphertext string     `json:"-"`
	SHA256            string     `json:"sha256,omitempty"`
	RowCount          int        `json:"row_count"`
	ErrorCode         string     `json:"error_code,omitempty"`
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
		return t.Omit("content_ciphertext").Where("operator_id=? AND requested_by=?", operator, actor).Order("created_at DESC").Limit(100).Find(&out).Error
	})
	return out, e
}
func (s *Service) RunExports(ctx context.Context, limit int) ([]Export, error) {
	if s.Source == nil {
		return nil, ErrNotReady
	}
	if limit < 1 || limit > 20 {
		limit = 5
	}
	jobs := []Export{}
	e := s.DB.WithContext(ctx).Transaction(func(t *gorm.DB) error {
		now := time.Now().UTC()
		if e := t.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Where("expires_at>? AND (state='pending' OR (state='processing' AND lease_until<?))", now, now).Order("created_at").Limit(limit).Find(&jobs).Error; e != nil {
			return e
		}
		for _, j := range jobs {
			if e := t.Model(&Export{}).Where("id=?", j.ID).Updates(map[string]any{"state": "processing", "lease_until": now.Add(time.Minute)}).Error; e != nil {
				return e
			}
		}
		return nil
	})
	if e != nil {
		return nil, e
	}
	for i := range jobs {
		j := &jobs[i]
		j.State = "ready"
		j.ErrorCode = ""
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
			j.ContentCiphertext, e = s.seal(b)
			return e
		}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
		if e != nil {
			j.State = "failed"
			j.ErrorCode = "source_unavailable"
			if e == ErrTooLarge {
				j.ErrorCode = "range_too_large"
			}
			j.ContentCiphertext = ""
		}
		if e = s.DB.WithContext(ctx).Model(&Export{}).Where("id=? AND state='processing'", j.ID).Updates(map[string]any{"state": j.State, "content_ciphertext": j.ContentCiphertext, "sha256": j.SHA256, "row_count": j.RowCount, "error_code": j.ErrorCode, "lease_until": nil}).Error; e != nil {
			return jobs, e
		}
		j.ContentCiphertext = ""
	}
	return jobs, nil
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
		if e := s.open(row.ContentCiphertext, &out); e != nil {
			return e
		}
		return t.Table("operations_export_access").Create(map[string]any{"id": uuid.NewString(), "export_id": id, "actor_id": actor, "at": time.Now().UTC()}).Error
	})
	return out, e
}
