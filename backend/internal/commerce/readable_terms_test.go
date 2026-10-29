package commerce

import (
	"strings"
	"testing"

	"github.com/tokpath/tokstudio/backend/internal/ledger"
)

func TestPostgresReadablePackageTermsPreserveRenewalConsentAndOrderSnapshot(t *testing.T) {
	f := setup(t)
	if f.quote.TermsSummary != DefaultPackageTerms || !strings.Contains(f.quote.TermsReference, f.quote.PackageID) || f.quote.TermsReference == "terms-v1" {
		t.Fatal("package still depends on an arbitrary reference", f.quote)
	}
	order := f.order(t, "accepted-readable-package", "0", "0")
	publish := func(version int64, terms string) PackageQuote {
		t.Helper()
		q, err := f.s.PublishPackage(f.ctx, nil, PublishPackageInput{ActorStaffID: f.owner, OperatorID: f.operator, PackageID: f.quote.PackageID, ExpectedVersion: version, Name: "Monthly", TermsSummary: terms, Timezone: "UTC", PriceMoney: d("80"), PaidATG: d("100"), GiftATG: d("20"), Period: ledger.Month, NewPurchaseEnabled: true, RenewalEnabled: true})
		if err != nil {
			t.Fatal(err)
		}
		return q
	}
	same := publish(1, "")
	if same.TermsReference == f.quote.TermsReference || RequiresRenewalConfirmation(f.quote, same) {
		t.Fatal("internal version change demanded consent for unchanged real terms", same)
	}
	changed := publish(2, DefaultPackageTerms+" 客户支持时间为工作日。")
	if !RequiresRenewalConfirmation(same, changed) {
		t.Fatal("changed customer terms bypassed renewal consent")
	}
	original, err := f.s.GetQuote(f.ctx, nil, *order.QuoteID)
	if err != nil || original.TermsSummary != DefaultPackageTerms || original.ID != f.quote.ID {
		t.Fatal("accepted package terms did not retain original quote", original, err)
	}
	if err = f.db.Model(&quoteRow{}).Where("id=?", original.ID).Update("terms_summary", "rewritten after acceptance").Error; err == nil {
		t.Fatal("accepted package terms could be rewritten")
	}
}
