package operations

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// These operational defaults match the existing risk form's hourly reminder
// and daily escalation cadence. They do not expire or release a restriction.
const restrictionReviewReminder = time.Hour
const restrictionReviewEscalation = 24 * time.Hour
const restrictionReviewKey = "restriction-review:"

type RestrictionReviewDirectory interface {
	ScopeFreezesDueForReview(context.Context, *gorm.DB, string, time.Time) ([]organization.ScopeFreeze, error)
	ScopeFreezeReviewStaff(context.Context, *gorm.DB, string, string, bool) ([]string, error)
	ScopeFreezeReviewCurrent(context.Context, *gorm.DB, string, string, string, string, string, int64) (bool, error)
}

type RestrictionReview struct {
	OperatorID     string `gorm:"primaryKey"`
	ScopeKind      string `gorm:"primaryKey"`
	ScopeID        string `gorm:"primaryKey"`
	Action         string `gorm:"primaryKey"`
	FreezeVersion  int64
	Status         string
	Sequence       int64
	LastNotifiedAt time.Time
}

func (RestrictionReview) TableName() string { return "operations_restriction_reviews" }

// RunRestrictionReviews records reminders and escalation independently of
// numeric risk rules. The exact decision version and reminder sequence provide
// durable deduplication across concurrent workers and maintenance-job retries.
func (s *Service) RunRestrictionReviews(ctx context.Context, operator string) ([]RestrictionReview, error) {
	if !validID(operator) {
		return nil, ErrInvalid
	}
	if s.Restrictions == nil {
		return nil, ErrNotReady
	}
	out := []RestrictionReview{}
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if e := lock(t, "restriction-reviews:"+operator); e != nil {
			return e
		}
		var now time.Time
		if e := t.Raw("SELECT clock_timestamp()").Row().Scan(&now); e != nil {
			return e
		}
		freezes, e := s.Restrictions.ScopeFreezesDueForReview(ctx, t, operator, now)
		if e != nil {
			return e
		}
		for _, f := range freezes {
			var state RestrictionReview
			if e = t.Where("operator_id=? AND scope_kind=? AND scope_id=? AND action=?", operator, f.ScopeKind, f.ScopeID, f.Action).Limit(1).Find(&state).Error; e != nil {
				return e
			}
			if state.FreezeVersion != f.Version {
				state = RestrictionReview{OperatorID: operator, ScopeKind: f.ScopeKind, ScopeID: f.ScopeID, Action: f.Action, FreezeVersion: f.Version}
			}
			severity := "warning"
			if now.Sub(*f.ReviewDueAt) >= restrictionReviewEscalation {
				severity = "critical"
			}
			if state.Status == severity && now.Sub(state.LastNotifiedAt) < restrictionReviewReminder {
				out = append(out, state)
				continue
			}
			recipients, e := s.Restrictions.ScopeFreezeReviewStaff(ctx, t, operator, f.ActorID, severity == "critical")
			if e != nil {
				return e
			}
			if len(recipients) == 0 {
				return ErrNotReady
			}
			state.Status = severity
			state.Sequence++
			state.LastNotifiedAt = now
			reference := f.ScopeKind + ":" + f.ScopeID + ":" + f.Action + ":" + strconv.FormatInt(f.Version, 10)
			title := "业务限制复核已超期"
			if severity == "critical" {
				title = "业务限制复核超期升级，请负责人处理"
			}
			seen := map[string]bool{}
			for _, id := range recipients {
				if seen[id] {
					continue
				}
				seen[id] = true
				_, e = s.Emit(ctx, t, Notification{OperatorID: operator, RecipientID: id, BusinessKey: restrictionReviewKey + reference + ":" + strconv.FormatInt(state.Sequence, 10), Category: "mandatory", Severity: severity, Title: title, Body: "范围：" + f.ScopeKind + " / " + f.ScopeID + "；限制事项：" + f.Action + "；复核期限：" + f.ReviewDueAt.UTC().Format(time.RFC3339) + "。请在业务限制页面核查并记录处理决定。限制不会因超期自动解除；未处理时每小时提醒，逾期24小时升级给本经营方负责人。", Reference: restrictionReviewKey + reference})
				if e != nil {
					return e
				}
			}
			if e = t.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "operator_id"}, {Name: "scope_kind"}, {Name: "scope_id"}, {Name: "action"}}, UpdateAll: true}).Create(&state).Error; e != nil {
				return e
			}
			out = append(out, state)
		}
		return nil
	})
	return out, e
}

func (s *Service) restrictionReviewCurrent(ctx context.Context, n Notification) (bool, error) {
	if s.Restrictions == nil {
		return false, ErrNotReady
	}
	parts := strings.Split(strings.TrimPrefix(n.Reference, restrictionReviewKey), ":")
	if !strings.HasPrefix(n.Reference, restrictionReviewKey) || len(parts) != 4 {
		return false, ErrInvalid
	}
	version, e := strconv.ParseInt(parts[3], 10, 64)
	if e != nil {
		return false, ErrInvalid
	}
	if !strings.HasPrefix(n.BusinessKey, n.Reference+":") {
		return false, ErrInvalid
	}
	sequence, e := strconv.ParseInt(strings.TrimPrefix(n.BusinessKey, n.Reference+":"), 10, 64)
	if e != nil || sequence < 1 {
		return false, ErrInvalid
	}
	var state RestrictionReview
	q := s.DB.WithContext(ctx).Where("operator_id=? AND scope_kind=? AND scope_id=? AND action=?", n.OperatorID, parts[0], parts[1], parts[2]).Limit(1).Find(&state)
	if q.Error != nil {
		return false, q.Error
	}
	// Keep every reminder in the inbox audit trail, but an SMTP outage must not
	// turn recovery into a backlog of superseded hourly warnings.
	if q.RowsAffected == 0 || state.FreezeVersion != version || state.Sequence != sequence || state.Status != n.Severity {
		return false, nil
	}
	return s.Restrictions.ScopeFreezeReviewCurrent(ctx, s.DB, n.OperatorID, n.RecipientID, parts[0], parts[1], parts[2], version)
}
