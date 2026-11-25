package usage

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
	"gorm.io/gorm"
)

const MaxImageBatchItems = 20

var batchCustomID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,79}$`)

type ImageBatchInput struct {
	ModelID  string
	TaskName string
	Items    []ImageBatchEntry
}

type ImageBatchEntry struct {
	CustomID string
	Request  modelruntime.Request
}

type ImageBatch struct {
	ID          string
	KeyID       string
	CustomerID  string
	BrandID     string
	BusinessKey string
	Fingerprint string
	ModelID     string
	TaskName    string
	CreatedAt   time.Time
	Items       []ImageBatchItem `gorm:"-"`
}

func (ImageBatch) TableName() string { return "usage_image_batches" }

type ImageBatchItem struct {
	BatchID         string
	CustomID        string
	Position        int
	RequestID       string
	Status          string
	Reason          string
	CancelRequested bool
	EstimateATG     ledger.Decimal
	ChargedATG      ledger.Decimal
	CompletedAt     *time.Time
}

func (ImageBatchItem) TableName() string { return "usage_image_batch_items" }

// Every item is a normal async image request. The batch, price snapshots,
// and all holds become visible to the existing worker in one commit.
func (s *Service) SubmitImageBatch(ctx context.Context, key Key, businessKey string, input ImageBatchInput) (ImageBatch, error) {
	if s.Runtime == nil || s.ResultStore == nil {
		return ImageBatch{}, ErrNotReady
	}
	input.TaskName = strings.TrimSpace(input.TaskName)
	if businessKey == "" || len(businessKey) > 150 || input.ModelID == "" || len([]rune(input.TaskName)) > 80 ||
		len(input.Items) == 0 || len(input.Items) > MaxImageBatchItems {
		return ImageBatch{}, modelruntime.ErrInvalidRequest
	}
	seen := make(map[string]bool, len(input.Items))
	for _, item := range input.Items {
		if !batchCustomID.MatchString(item.CustomID) || seen[item.CustomID] || item.Request.ID != "" ||
			item.Request.PublicModel != input.ModelID || !item.Request.Operation.IsImage() || item.Request.Stream {
			return ImageBatch{}, modelruntime.ErrInvalidRequest
		}
		seen[item.CustomID] = true
	}
	batch := ImageBatch{
		ID: uuid.NewString(), KeyID: key.ID, CustomerID: key.CustomerID, BrandID: key.BrandID,
		BusinessKey: businessKey, Fingerprint: s.fingerprint(input), ModelID: input.ModelID, TaskName: input.TaskName,
	}
	err := s.within(ctx, nil, func(tx *gorm.DB) error {
		var err error
		batch.CreatedAt, err = s.businessTime(tx)
		if err != nil {
			return err
		}
		// The key FK acquires a row lock. Reserve the normal key/catalog locks
		// before inserting the batch, in the same order as standalone admissions.
		// A business-key advisory lock serializes only replays of this batch.
		if err = tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "image-batch:"+key.ID+":"+businessKey).Error; err != nil {
			return err
		}
		var existing ImageBatch
		err = tx.Where("key_id=? AND business_key=?", key.ID, businessKey).Take(&existing).Error
		if err == nil {
			if existing.Fingerprint != batch.Fingerprint || existing.CustomerID != key.CustomerID || existing.BrandID != key.BrandID {
				return ErrConflict
			}
			batch = existing
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		items := make([]map[string]any, 0, len(input.Items))
		for position, item := range input.Items {
			req := item.Request
			req.Async = true
			accepted, created, err := s.admit(ctx, tx, key, "image-batch:"+batch.ID+":"+strconv.Itoa(position), req, batch.CreatedAt)
			if err != nil {
				return err
			}
			if !created {
				return ErrConflict
			}
			items = append(items, map[string]any{
				"batch_id": batch.ID, "custom_id": item.CustomID, "position": position, "request_id": accepted.ID,
			})
		}
		// Durable request counts already cover every item under the locked Key.
		// Probe Redis once without inserting counters for a batch that may roll back.
		if s.AdmissionLimiter != nil {
			if err = s.AdmissionLimiter.Check(ctx, key.ID, batch.ID, nil, 0); err != nil {
				return err
			}
		}
		if err = tx.Create(&batch).Error; err != nil {
			return err
		}
		return tx.Table((ImageBatchItem{}).TableName()).Create(items).Error
	})
	if err != nil {
		return ImageBatch{}, err
	}
	return s.ReadImageBatch(ctx, key, batch.ID)
}

func (s *Service) imageBatches(ctx context.Context, key Key) *gorm.DB {
	return s.DB.WithContext(ctx).Model(&ImageBatch{}).
		Where("key_id=? AND customer_id=? AND brand_id=?", key.ID, key.CustomerID, key.BrandID)
}

func (s *Service) ReadImageBatch(ctx context.Context, key Key, id string) (ImageBatch, error) {
	var batch ImageBatch
	if _, err := uuid.Parse(id); err != nil {
		return batch, modelruntime.ErrInvalidRequest
	}
	if err := s.imageBatches(ctx, key).Take(&batch, "id=?", id).Error; err != nil {
		return batch, err
	}
	items, err := s.imageBatchItems(ctx, []string{batch.ID})
	batch.Items = items[batch.ID]
	return batch, err
}

func (s *Service) ListImageBatches(ctx context.Context, key Key, cursor string, limit int) ([]ImageBatch, string, error) {
	if limit < 1 || limit > 50 {
		return nil, "", modelruntime.ErrInvalidRequest
	}
	q := s.imageBatches(ctx, key)
	if cursor != "" {
		if _, err := uuid.Parse(cursor); err != nil {
			return nil, "", modelruntime.ErrInvalidRequest
		}
		// Resolve cursors only inside this key's scope.
		var after ImageBatch
		if err := s.imageBatches(ctx, key).Take(&after, "id=?", cursor).Error; err != nil {
			return nil, "", err
		}
		q = q.Where("(created_at,id)<(?,?)", after.CreatedAt, after.ID)
	}
	rows := []ImageBatch{}
	if err := q.Order("created_at DESC,id DESC").Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, "", err
	}
	next := ""
	if len(rows) > limit {
		rows = rows[:limit]
		next = rows[len(rows)-1].ID
	}
	ids := make([]string, len(rows))
	for i, batch := range rows {
		ids[i] = batch.ID
	}
	items, err := s.imageBatchItems(ctx, ids)
	for i := range rows {
		rows[i].Items = items[rows[i].ID]
	}
	return rows, next, err
}

func (s *Service) imageBatchItems(ctx context.Context, ids []string) (map[string][]ImageBatchItem, error) {
	items := make(map[string][]ImageBatchItem, len(ids))
	if len(ids) == 0 {
		return items, nil
	}
	var rows []ImageBatchItem
	err := s.DB.WithContext(ctx).Table("usage_image_batch_items i").
		Select("i.*,r.status,r.reason,r.cancel_requested,r.estimate_atg,r.charged_atg,r.completed_at").
		Joins("JOIN usage_requests r ON r.id=i.request_id").
		Where("i.batch_id IN ?", ids).Order("i.position").Scan(&rows).Error
	for _, row := range rows {
		items[row.BatchID] = append(items[row.BatchID], row)
	}
	return items, err
}

func (s *Service) CancelImageBatch(ctx context.Context, key Key, id string) (ImageBatch, error) {
	if _, err := uuid.Parse(id); err != nil {
		return ImageBatch{}, modelruntime.ErrInvalidRequest
	}
	if err := s.DB.WithContext(ctx).Exec(`UPDATE usage_requests r SET cancel_requested=true
		FROM usage_image_batches b JOIN usage_image_batch_items i ON i.batch_id=b.id
		WHERE b.id=? AND b.key_id=? AND b.customer_id=? AND b.brand_id=?
		AND r.id=i.request_id AND r.status IN ('accepted','executing','submitted','awaiting_usage')`,
		id, key.ID, key.CustomerID, key.BrandID).Error; err != nil {
		return ImageBatch{}, err
	}
	return s.ReadImageBatch(ctx, key, id)
}
