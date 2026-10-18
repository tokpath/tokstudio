package organization

import (
	"context"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func ValidateRelationship(parent Operator, kind OperatorKind) error {
	if (parent.Kind == Platform && (kind == OEM || kind == Channel)) || (parent.Kind == OEM && kind == Channel) {
		return nil
	}
	return ErrInvalidRelationship
}

// BootstrapPlatform is a one-time installation command. PostgreSQL unique
// indexes serialize competing installers and prevent a second official brand.
func (s *Service) BootstrapPlatform(ctx context.Context, tx *gorm.DB, in BootstrapInput) (out OperatorCreated, err error) {
	if strings.TrimSpace(in.BrandName) == "" || strings.TrimSpace(in.OperatorName) == "" || !validID(in.OwnerStaffID) {
		return out, ErrInvalidInput
	}
	err = s.withTx(ctx, tx, func(tx *gorm.DB) error {
		now := s.now()
		opID, brandID := newID(), newID()
		b := brandRow{ID: brandID, Kind: "official", Name: strings.TrimSpace(in.BrandName), OwnerOperatorID: opID, Status: "active", CreatedAt: now}
		o := operatorRow{ID: opID, BrandID: brandID, Kind: Platform, Name: strings.TrimSpace(in.OperatorName), LegalName: in.LegalName, Lifecycle: "active", CreatedAt: now}
		if err := tx.Create(&b).Error; err != nil {
			return err
		}
		if err := tx.Create(&o).Error; err != nil {
			return err
		}
		st := businessRow{OperatorID: opID, Settlement: true, Version: 1, UpdatedAt: now}
		if err := tx.Create(&st).Error; err != nil {
			return err
		}
		m := membershipRow{ID: newID(), OperatorID: opID, StaffID: in.OwnerStaffID, Role: "owner", Status: "active", CreatedAt: now}
		if err := tx.Create(&m).Error; err != nil {
			return err
		}
		out = OperatorCreated{Operator: Operator(o), Brand: Brand(b)}
		return s.event(tx, opID, in.OwnerStaffID, "platform.bootstrapped", "installation", out)
	})
	return
}

func (s *Service) CreateOperator(ctx context.Context, tx *gorm.DB, in CreateOperatorInput) (out OperatorCreated, err error) {
	if !validID(in.ParentOperatorID) || strings.TrimSpace(in.Name) == "" || len(in.SupplyTerms) > 10000 {
		return out, ErrInvalidInput
	}
	err = s.withTx(ctx, tx, func(tx *gorm.DB) error {
		if err := s.RequirePermission(ctx, tx, in.ActorStaffID, in.ParentOperatorID, "organization.create"); err != nil {
			return err
		}
		p, err := s.GetOperator(ctx, tx, in.ParentOperatorID)
		if err != nil {
			return err
		}
		if err = ValidateRelationship(p, in.Kind); err != nil {
			return err
		}
		if p.Lifecycle != "active" {
			return ErrBusinessClosed
		}
		if in.ExitPolicy == "" {
			in.ExitPolicy = "non_refundable"
		}
		if in.ExitPolicy != "non_refundable" && in.ExitPolicy != "remaining_paid_proportional" {
			return ErrInvalidInput
		}
		in.ContractAttachmentID = strings.TrimSpace(in.ContractAttachmentID)
		if err = s.RequireSupplyContract(ctx, tx, p.ID, in.ContractAttachmentID); err != nil {
			return err
		}
		now := s.now()
		opID := newID()
		b, err := s.GetBrand(ctx, tx, p.BrandID)
		if err != nil {
			return err
		}
		if in.Kind == OEM {
			if strings.TrimSpace(in.BrandName) == "" {
				return ErrInvalidInput
			}
			b = Brand{ID: newID(), Kind: "oem", Name: strings.TrimSpace(in.BrandName), OwnerOperatorID: opID, Status: "active", CreatedAt: now}
			row := brandRow(b)
			if err = tx.Create(&row).Error; err != nil {
				return err
			}
		}
		pID := p.ID
		o := operatorRow{ID: opID, BrandID: b.ID, ParentOperatorID: &pID, Kind: in.Kind, Name: strings.TrimSpace(in.Name), LegalName: in.LegalName, Lifecycle: "draft", CreatedAt: now}
		if err = tx.Create(&o).Error; err != nil {
			return err
		}
		st := businessRow{OperatorID: opID, Settlement: true, Version: 1, UpdatedAt: now}
		if err = tx.Create(&st).Error; err != nil {
			return err
		}
		agreementID := newID()
		a := supplyRow{ID: agreementID, SupplierID: p.ID, BuyerID: opID, Status: "active", SupplyTerms: strings.TrimSpace(in.SupplyTerms), ExitPolicy: in.ExitPolicy, ContractAttachmentID: in.ContractAttachmentID, Version: 1, CreatedAt: now}
		if err = tx.Create(&a).Error; err != nil {
			return err
		}
		out = OperatorCreated{Operator: Operator(o), Brand: b, Supply: (*SupplyAgreement)(&a)}
		if in.Kind == Channel {
			code, err := inviteCode()
			if err != nil {
				return err
			}
			if err = tx.Exec(`INSERT INTO organization_invite_codes(id,code,brand_id,kind,operator_id,enabled,created_at) VALUES(?,?,?,'channel',?,true,?)`, newID(), code, b.ID, opID, now).Error; err != nil {
				return err
			}
			out.InviteCode = code
		}
		return s.event(tx, p.ID, in.ActorStaffID, "operator.created", "", out)
	})
	return
}

func (s *Service) GetSupplyAgreement(ctx context.Context, tx *gorm.DB, buyerID string) (SupplyAgreement, error) {
	if !validID(buyerID) {
		return SupplyAgreement{}, ErrInvalidInput
	}
	var r supplyRow
	err := s.dbFor(ctx, tx).First(&r, "buyer_id = ?", buyerID).Error
	return SupplyAgreement(r), classify(err)
}
func (s *Service) SetSupplyStatus(ctx context.Context, tx *gorm.DB, actorID, buyerID, status, evidence string) error {
	if status != "active" && status != "paused" && status != "exited" {
		return ErrInvalidInput
	}
	if strings.TrimSpace(evidence) == "" {
		return ErrInvalidInput
	}
	return s.withTx(ctx, tx, func(tx *gorm.DB) error {
		var a supplyRow
		if err := tx.First(&a, "buyer_id = ?", buyerID).Error; err != nil {
			return err
		}
		if err := s.RequirePermission(ctx, tx, actorID, a.SupplierID, "cooperation.manage"); err != nil {
			return err
		}
		if status == "exited" {
			// This is the same outer fence as channel takeover. Settlement facts
			// hold SHARE before entering payment rows, preventing a missed receipt
			// between this readiness check and the exit transition.
			var op operatorRow
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&op, "id=?", buyerID).Error; err != nil {
				return err
			}
			if s.exitChecker == nil {
				return ErrReadiness
			}
			checks, err := s.exitChecker.CheckOperatorExit(ctx, tx, buyerID)
			if err != nil {
				return err
			}
			pendingReported := false
			missing := []Requirement{}
			for _, r := range checks {
				if r.Code == "pending_receipts" {
					pendingReported = true
				}
				if !r.Ready {
					missing = append(missing, r)
				}
			}
			if !pendingReported {
				missing = append(missing, Requirement{Code: "pending_receipts", Description: "actual original-currency receipt obligations were not checked"})
			}
			if len(missing) > 0 {
				return &ReadinessError{Missing: missing}
			}
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&a, "buyer_id = ?", buyerID).Error; err != nil {
			return err
		}
		if a.Status == "exited" && status != "exited" {
			return ErrConflict
		}
		if err := tx.Model(&a).Update("status", status).Error; err != nil {
			return err
		}
		return s.event(tx, a.SupplierID, actorID, "supply.status_changed", evidence, map[string]any{"buyer_id": buyerID, "status": status})
	})
}

func validFeature(f BusinessFeature) bool {
	switch f {
	case Registration, NewPurchase, Renewal, ModelUsage, Activity, Settlement:
		return true
	}
	return false
}
func (s *Service) GetBusinessStatus(ctx context.Context, tx *gorm.DB, operatorID string) (BusinessStatus, error) {
	if !validID(operatorID) {
		return BusinessStatus{}, ErrInvalidInput
	}
	var r businessRow
	err := s.dbFor(ctx, tx).First(&r, "operator_id = ?", operatorID).Error
	return BusinessStatus(r), classify(err)
}
func (s *Service) RequireBusiness(ctx context.Context, tx *gorm.DB, operatorID string, feature BusinessFeature) error {
	if !validFeature(feature) {
		return ErrInvalidInput
	}
	o, err := s.GetOperator(ctx, tx, operatorID)
	if err != nil {
		return err
	}
	st, err := s.GetBusinessStatus(ctx, tx, operatorID)
	if err != nil {
		return err
	}
	enabled := map[BusinessFeature]bool{Registration: st.Registration, NewPurchase: st.NewPurchase, Renewal: st.Renewal, ModelUsage: st.ModelUsage, Activity: st.Activity, Settlement: st.Settlement}[feature]
	if !enabled {
		return ErrBusinessClosed
	}
	if feature == Settlement {
		return nil
	} // Pausing sales does not prevent settlement.
	b, err := s.GetBrand(ctx, tx, o.BrandID)
	if err != nil {
		return err
	}
	if o.Lifecycle != "active" || b.Status != "active" {
		return ErrBusinessClosed
	}
	if o.Kind != Platform {
		a, err := s.GetSupplyAgreement(ctx, tx, o.ID)
		if err != nil {
			return err
		}
		if a.Status != "active" {
			return ErrBusinessClosed
		}
	}
	return s.RequireUnfrozen(ctx, tx, o.BrandID, o.ID, "", string(feature))
}

func (s *Service) SetBusinessStatus(ctx context.Context, tx *gorm.DB, in SetBusinessStatusInput, checker ReadinessChecker) (out BusinessStatus, err error) {
	if !validFeature(in.Feature) || in.ExpectedVersion < 1 || strings.TrimSpace(in.EvidenceReference) == "" {
		return out, ErrInvalidInput
	}
	err = s.withTx(ctx, tx, func(tx *gorm.DB) error {
		if err := s.RequirePermission(ctx, tx, in.ActorStaffID, in.OperatorID, "business.manage"); err != nil {
			return err
		}
		// Commands that stop registration and channel takeover share this
		// operator lock with final registration, giving a single linearization point.
		var locked operatorRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, "id = ?", in.OperatorID).Error; err != nil {
			return err
		}
		var st businessRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&st, "operator_id = ?", in.OperatorID).Error; err != nil {
			return err
		}
		if st.Version != in.ExpectedVersion {
			return ErrConflict
		}
		o, err := s.GetOperator(ctx, tx, in.OperatorID)
		if err != nil {
			return err
		}
		if in.Enabled && in.Feature != Settlement {
			b, err := s.GetBrand(ctx, tx, o.BrandID)
			if err != nil {
				return err
			}
			missing := []Requirement{}
			if o.Lifecycle != "active" {
				missing = append(missing, Requirement{Code: "operator_active", Description: "负责人尚未开通经营方"})
			}
			if b.Status != "active" {
				missing = append(missing, Requirement{Code: "brand_active", Description: "品牌未开放"})
			}
			if o.Kind != Platform {
				a, err := s.GetSupplyAgreement(ctx, tx, o.ID)
				if err != nil {
					return err
				}
				if a.Status != "active" {
					missing = append(missing, Requirement{Code: "supply_active", Description: "直接供货合作未开放"})
				}
			}
			if checker != nil {
				reqs, err := checker.Check(ctx, tx, o, in.Feature)
				if err != nil {
					return err
				}
				for _, r := range reqs {
					if !r.Ready {
						missing = append(missing, r)
					}
				}
			} else if in.Feature != Registration {
				missing = append(missing, Requirement{Code: "readiness_adapter", Description: "尚未配置该业务的开通检查"})
			}
			if len(missing) > 0 {
				return &ReadinessError{Missing: missing}
			}
		}
		if err = tx.Model(&businessRow{}).Where("operator_id = ? AND version = ?", o.ID, st.Version).Updates(map[string]any{string(in.Feature): in.Enabled, "version": st.Version + 1, "updated_at": s.now()}).Error; err != nil {
			return err
		}
		out, err = s.GetBusinessStatus(ctx, tx, o.ID)
		if err != nil {
			return err
		}
		return s.event(tx, o.ID, in.ActorStaffID, "business.status_changed", in.EvidenceReference, in)
	})
	return
}

// RequireSupplyContract accepts only an immutable private attachment uploaded by
// the supplier. An arbitrary URL, object key, finance evidence, or another
// operator's attachment is never a cooperation contract.
func (s *Service) RequireSupplyContract(ctx context.Context, tx *gorm.DB, supplierID, attachmentID string) error {
	if attachmentID == "" {
		return nil
	}
	if !validID(attachmentID) || !validID(supplierID) {
		return ErrInvalidInput
	}
	var count int64
	if err := s.dbFor(ctx, tx).Table("organization_cooperation_contracts").Where("id=? AND operator_id=? AND status='ready'", attachmentID, supplierID).Count(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return ErrForbidden
	}
	return nil
}
