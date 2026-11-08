package commerce

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
)

type TakeoverPackage struct {
	ID         string         `json:"id"`
	QuoteID    string         `json:"quote_id"`
	Name       string         `json:"name"`
	PriceMoney ledger.Decimal `json:"price_money"`
	PaidATG    ledger.Decimal `json:"paid_atg"`
	GiftATG    ledger.Decimal `json:"gift_atg"`
	Period     ledger.Period  `json:"period"`
}

func (s *Service) TakeoverPackages(ctx context.Context, tx *gorm.DB, operator string) ([]TakeoverPackage, error) {
	rows := []TakeoverPackage{}
	e := s.dbFor(ctx, tx).Table("commerce_packages p").Select("p.id,p.name,q.id AS quote_id,q.price_money,q.paid_atg,q.gift_atg,q.period").Joins("JOIN commerce_package_quotes q ON q.id=p.current_quote_id").Where("p.operator_id=?", operator).Order("p.id").Scan(&rows).Error
	return rows, e
}

// Copy the current catalog once, keeping original orders and quote versions
// untouched. Imported products start with new sales and renewals disabled.
func (s *Service) copyTakeoverPackages(ctx context.Context, t *gorm.DB, in TransferInput) error {
	var packages []packageRow
	if e := t.Where("operator_id=?", in.FromOperatorID).Order("id").Find(&packages).Error; e != nil {
		return e
	}
	for _, source := range packages {
		if source.CurrentQuoteID == nil {
			continue
		}
		var quote quoteRow
		if e := t.First(&quote, "id=?", *source.CurrentQuoteID).Error; e != nil {
			return e
		}
		id, qid := uuid.NewString(), uuid.NewString()
		copy := packageRow{ID: id, OperatorID: in.ToOperatorID, Name: source.Name, SourceOperatorID: &in.FromOperatorID, SourcePackageID: &source.ID, TakeoverID: &in.TakeoverID, CreatedAt: s.now()}
		if e := t.Create(&copy).Error; e != nil {
			return e
		}
		quote.ID, quote.PackageID, quote.OperatorID, quote.Version = qid, id, in.ToOperatorID, 1
		quote.TermsReference = fmt.Sprintf("package-terms:%s:v1", id)
		quote.CreatedAt = s.now()
		if e := t.Create(&quote).Error; e != nil {
			return e
		}
		if e := t.Model(&copy).Update("current_quote_id", qid).Error; e != nil {
			return e
		}
		if e := t.Model(&subscriptionRow{}).Where("operator_id=? AND package_id=?", in.FromOperatorID, source.ID).Update("package_id", id).Error; e != nil {
			return e
		}
	}
	return nil
}
