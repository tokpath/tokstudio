package cooperation

import (
	"context"
	"gorm.io/gorm"
)

func (s *Service) ListBuyerQuotes(ctx context.Context, tx *gorm.DB, actor, buyer string) ([]PurchaseQuote, error) {
	out := []PurchaseQuote{}
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		if e := s.Organization.RequirePermission(ctx, t, actor, buyer, "cooperation.manage"); e != nil {
			return e
		}
		return t.Where("buyer_id=?", buyer).Order("created_at DESC,id").Limit(100).Find(&out).Error
	})
	return out, e
}
func (s *Service) ListSupplierQuotes(ctx context.Context, tx *gorm.DB, actor, supplier string) ([]PurchaseQuote, error) {
	out := []PurchaseQuote{}
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		if e := s.Organization.RequirePermission(ctx, t, actor, supplier, "cooperation.manage"); e != nil {
			return e
		}
		return t.Where("supplier_id=?", supplier).Order("created_at DESC,id").Limit(100).Find(&out).Error
	})
	return out, e
}
func (s *Service) GetPurchaseQuote(ctx context.Context, tx *gorm.DB, actor, operator, id string) (PurchaseQuote, error) {
	var out PurchaseQuote
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		if e := s.Organization.RequirePermission(ctx, t, actor, operator, "cooperation.manage"); e != nil {
			return e
		}
		return t.First(&out, "id=? AND (buyer_id=? OR supplier_id=?)", id, operator, operator).Error
	})
	return out, e
}
func (s *Service) ListPurchaseOrders(ctx context.Context, tx *gorm.DB, actor, operator, side string, limit int) ([]PurchaseOrder, error) {
	if side != "buyer" && side != "supplier" {
		return nil, ErrConflict
	}
	if limit < 1 || limit > 100 {
		limit = 50
	}
	out := []PurchaseOrder{}
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		if e := s.Organization.RequirePermission(ctx, t, actor, operator, "cooperation.manage"); e != nil {
			return e
		}
		q := t.Where("buyer_id=?", operator)
		if side == "supplier" {
			q = t.Where("supplier_id=?", operator)
		}
		return q.Order("created_at DESC,id").Limit(limit).Find(&out).Error
	})
	return out, e
}
func (s *Service) GetPurchase(ctx context.Context, tx *gorm.DB, actor, operator, id string) (PurchaseOrder, error) {
	var out PurchaseOrder
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		if e := s.Organization.RequirePermission(ctx, t, actor, operator, "cooperation.manage"); e != nil {
			return e
		}
		return t.First(&out, "id=? AND (buyer_id=? OR supplier_id=?)", id, operator, operator).Error
	})
	return out, e
}
func (s *Service) ListProcurementLots(ctx context.Context, tx *gorm.DB, actor, buyer string) ([]ProcurementLot, error) {
	var out []ProcurementLot
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		if e := s.Organization.RequirePermission(ctx, t, actor, buyer, "cooperation.manage"); e != nil {
			return e
		}
		return t.Where("buyer_id=?", buyer).Order("delivered_at DESC,id").Limit(100).Find(&out).Error
	})
	return out, e
}
func (s *Service) ListProcurementExits(ctx context.Context, tx *gorm.DB, actor, operator string) ([]ProcurementExit, error) {
	out := []ProcurementExit{}
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		if e := s.Organization.RequirePermission(ctx, t, actor, operator, "cooperation.manage"); e != nil {
			return e
		}
		return t.Where("buyer_id=? OR supplier_id=?", operator, operator).Order("created_at DESC,id").Limit(100).Find(&out).Error
	})
	return out, e
}
