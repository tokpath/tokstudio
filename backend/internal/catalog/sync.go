package catalog

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

const (
	SyncDraft     = "draft"
	SyncReviewed  = "reviewed"
	SyncRejected  = "rejected"
	SyncPublished = "published"
)

func (s *Service) PublishModel(ctx context.Context, publicID string) (*ModelView, error) {
	model, err := s.loadModel(ctx, publicID)
	if err != nil {
		return nil, err
	}
	if err := s.validateModelReady(ctx, *model); err != nil {
		return nil, err
	}
	if model.Status == SyncPublished {
		return s.modelView(ctx, *model)
	}
	if err := s.db.WithContext(ctx).Model(&publicModelRow{}).Where("id = ?", model.ID).
		Updates(map[string]any{"status": SyncPublished, "sync_state": SyncPublished}).Error; err != nil {
		return nil, err
	}
	model.Status = SyncPublished
	model.SyncState = SyncPublished
	return s.modelView(ctx, *model)
}

func (s *Service) DeprecateModel(ctx context.Context, publicID string) (*ModelView, error) {
	model, err := s.loadModel(ctx, publicID)
	if err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&publicModelRow{}).Where("id = ?", model.ID).Update("status", "deprecated").Error; err != nil {
			return err
		}
		return tx.Model(&routeGroupRow{}).Where("public_model_id = ?", model.ID).Update("status", "inactive").Error
	}); err != nil {
		return nil, err
	}
	model.Status = "deprecated"
	return s.modelView(ctx, *model)
}

func (s *Service) loadModel(ctx context.Context, publicID string) (*publicModelRow, error) {
	var model publicModelRow
	if err := s.db.WithContext(ctx).Where("id = ? OR public_id = ?", publicID, publicID).First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUnknownModel
		}
		return nil, err
	}
	return &model, nil
}
