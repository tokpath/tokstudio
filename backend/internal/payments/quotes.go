package payments

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"golang.org/x/text/currency"
	"gorm.io/gorm"
)

var ErrQuoteUnavailable = errors.New("payment currency quote unavailable")

// PaymentQuote prices one payment. It never rewrites asset balances, upstream
// costs, or accounting currency. The original receipt remains original money.
type PaymentQuoter interface {
	QuotePayment(context.Context, Merchant, Order) (PaymentQuote, error)
}

type PaymentQuote struct {
	InvoiceMoney    *Money         `json:"invoice_money,omitempty"`
	RateNumerator   string         `json:"rate_numerator,omitempty"`
	RateDenominator string         `json:"rate_denominator,omitempty"`
	ID              string         `json:"id"`
	SettlementMoney Money          `json:"settlement_money"`
	PayMoney        Money          `json:"pay_money"`
	Rate            ledger.Decimal `json:"rate"`
	Source          string         `json:"source"`
	SourceURL       string         `json:"source_url"`
	RateDate        time.Time      `json:"rate_date"`
	QuotedAt        time.Time      `json:"quoted_at"`
	ExpiresAt       time.Time      `json:"expires_at"`
}

type FXSettings struct {
	LastAttemptAt *time.Time `json:"last_attempt_at,omitempty"`
	ErrorCode     string     `json:"error_code,omitempty"`
	Source        string     `json:"source"`
	Enabled       bool       `json:"enabled"`
	Version       int64      `json:"version"`
	UpdatedAt     time.Time  `json:"updated_at"`
	LastSuccessAt *time.Time `json:"last_success_at" gorm:"-"`
	RateDate      *time.Time `json:"rate_date" gorm:"-"`
	Status        string     `json:"status" gorm:"-"`
}

func (s *Service) FXSettings(ctx context.Context) (FXSettings, error) {
	var out FXSettings
	if err := s.DB.WithContext(ctx).Table("payments_fx_settings").Where("id=true").Take(&out).Error; err != nil {
		return out, err
	}
	var rate struct {
		FetchedAt, RateDate *time.Time
		ErrorCode           string
	}
	if err := s.DB.WithContext(ctx).Table("payments_fx_rates").Order("attempted_at DESC").Limit(1).Find(&rate).Error; err != nil {
		return out, err
	}
	out.LastSuccessAt, out.RateDate = rate.FetchedAt, rate.RateDate
	out.Status = "not_synced"
	if !out.Enabled {
		out.Status = "disabled"
	} else if rate.ErrorCode != "" || out.ErrorCode != "" {
		out.Status = "unavailable"
	} else if rate.FetchedAt != nil {
		out.Status = "ready"
		if time.Since(*rate.FetchedAt) > 24*time.Hour {
			out.Status = "stale"
		}
	}
	return out, nil
}

func (s *Service) SetFXSettings(ctx context.Context, actor, platform string, in FXSettings, expected int64) (FXSettings, error) {
	if s.Authorize == nil || in.Source != "frankfurter" {
		return FXSettings{}, ErrInvalid
	}
	if err := s.Authorize.RequirePermission(ctx, s.DB, actor, platform, "payments.manage"); err != nil {
		return FXSettings{}, err
	}
	r := s.DB.WithContext(ctx).Table("payments_fx_settings").Where("id=true AND version=?", expected).Updates(map[string]any{"source": in.Source, "enabled": in.Enabled, "version": expected + 1, "updated_at": gorm.Expr("clock_timestamp()")})
	if r.Error != nil {
		return FXSettings{}, r.Error
	}
	if r.RowsAffected != 1 {
		return FXSettings{}, ErrConflict
	}
	return s.FXSettings(ctx)
}

type fxRateRow struct {
	BaseCurrency, QuoteCurrency      string
	Rate                             ledger.Decimal
	RateDate, FetchedAt, AttemptedAt time.Time
	ErrorCode                        string
}

func (s *Service) referenceRate(ctx context.Context, base, quote string, force bool) (fxRateRow, error) {
	settings, err := s.FXSettings(ctx)
	if err != nil {
		return fxRateRow{}, err
	}
	if !settings.Enabled {
		return fxRateRow{}, ErrQuoteUnavailable
	}
	var cached fxRateRow
	if err = s.DB.WithContext(ctx).Table("payments_fx_rates").Where("base_currency=? AND quote_currency=?", base, quote).Limit(1).Find(&cached).Error; err != nil {
		return cached, err
	}
	now := time.Now().UTC()
	usable := cached.Rate.Sign() > 0 && now.Sub(cached.FetchedAt) < 24*time.Hour && now.Sub(cached.RateDate) < 7*24*time.Hour
	if !force && usable && now.Sub(cached.AttemptedAt) < time.Hour {
		return cached, nil
	}
	if s.FXTransport == nil {
		return cached, ErrQuoteUnavailable
	}
	endpoint, err := s.FXTransport.endpoint(Merchant{APIBaseURL: s.FXBaseURL}, "https://api.frankfurter.dev")
	if err != nil {
		return cached, err
	}
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	raw, _, err := s.FXTransport.do(callCtx, http.MethodGet, endpoint+"/v2/rate/"+strings.ToLower(base)+"/"+strings.ToLower(quote), nil, http.Header{"Accept": []string{"application/json"}})
	var result struct {
		Base  string      `json:"base"`
		Quote string      `json:"quote"`
		Date  string      `json:"date"`
		Rate  json.Number `json:"rate"`
	}
	if err == nil {
		err = json.Unmarshal(raw, &result)
	}
	var rate ledger.Decimal
	var date time.Time
	if err == nil {
		rate, err = ledger.ParseDecimal(result.Rate.String())
	}
	if err == nil {
		date, err = time.Parse("2006-01-02", result.Date)
	}
	if err == nil && (!strings.EqualFold(result.Base, base) || !strings.EqualFold(result.Quote, quote) || rate.Sign() <= 0 || date.After(now.Truncate(24*time.Hour)) || now.Sub(date) >= 7*24*time.Hour) {
		err = ErrQuoteUnavailable
	}
	if err != nil {
		_ = s.DB.WithContext(context.WithoutCancel(ctx)).Table("payments_fx_settings").Where("id=true").Updates(map[string]any{"last_attempt_at": now, "error_code": "source_unavailable"}).Error
		if cached.Rate.Sign() > 0 {
			_ = s.DB.WithContext(context.WithoutCancel(ctx)).Table("payments_fx_rates").Where("base_currency=? AND quote_currency=?", base, quote).Updates(map[string]any{"attempted_at": now, "error_code": "source_unavailable"}).Error
		}
		if usable {
			return cached, nil
		}
		return cached, ErrQuoteUnavailable
	}
	row := fxRateRow{BaseCurrency: base, QuoteCurrency: quote, Rate: rate, RateDate: date, FetchedAt: now, AttemptedAt: now}
	err = s.DB.WithContext(ctx).Exec("INSERT INTO payments_fx_rates(base_currency,quote_currency,rate,rate_date,fetched_at,attempted_at) VALUES(?,?,?,?,?,?) ON CONFLICT(base_currency,quote_currency) DO UPDATE SET rate=excluded.rate,rate_date=excluded.rate_date,fetched_at=excluded.fetched_at,attempted_at=excluded.attempted_at,error_code=''", base, quote, rate, date, now, now).Error
	if err == nil {
		err = s.DB.WithContext(ctx).Table("payments_fx_settings").Where("id=true").Updates(map[string]any{"last_attempt_at": now, "error_code": ""}).Error
	}
	return row, err
}

// Refresh only currency pairs already used by the service. No background
// Internet dependency is introduced for native-currency installations.
func (s *Service) RefreshQuoteRates(ctx context.Context) error {
	var rows []fxRateRow
	if err := s.DB.WithContext(ctx).Table("payments_fx_rates").Where("attempted_at<clock_timestamp()-interval '1 hour'").Order("attempted_at").Limit(5).Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		if _, err := s.referenceRate(ctx, row.BaseCurrency, row.QuoteCurrency, false); err != nil && !errors.Is(err, ErrQuoteUnavailable) {
			return err
		}
	}
	return nil
}

func quoteMoney(o Order, rate ledger.Decimal, source, sourceURL string, date time.Time) (PaymentQuote, error) {
	return quoteMoneyRat(o, rate.Rat(), source, sourceURL, date)
}

func fiatMinorDigits(code string) (uint8, bool) {
	unit, err := currency.ParseISO(code)
	if err != nil || code == "XXX" || code == "XTS" {
		return 0, false
	}
	scale, _ := currency.Standard.Rounding(unit)
	return uint8(scale), scale >= 0 && scale <= 18
}

func quoteMoneyRat(o Order, ratio *big.Rat, source, sourceURL string, date time.Time) (PaymentQuote, error) {
	rate, _ := ledger.Quantize(ratio)
	digits, ok := fiatMinorDigits(o.Money.Currency)
	if !ok || rate.Sign() <= 0 {
		return PaymentQuote{}, ErrUnsupported
	}
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(digits)), nil)
	exact := new(big.Rat).Mul(o.Settlement().Amount.Rat(), ratio)
	exact.Mul(exact, new(big.Rat).SetInt(scale))
	units := new(big.Int).Quo(exact.Num(), exact.Denom())
	if !exact.IsInt() {
		units.Add(units, big.NewInt(1))
	}
	amount, remainder := ledger.Quantize(new(big.Rat).SetFrac(units, scale))
	if remainder.Sign() != 0 || amount.Sign() <= 0 {
		return PaymentQuote{}, ErrInvalid
	}
	now := time.Now().UTC()
	expires := o.ExpiresAt
	if !expires.After(now) {
		return PaymentQuote{}, ErrInvalid
	}
	return PaymentQuote{RateNumerator: ratio.Num().String(), RateDenominator: ratio.Denom().String(), ID: uuid.NewString(), SettlementMoney: o.Settlement(), PayMoney: Money{Currency: o.Money.Currency, Amount: amount}, Rate: rate, Source: source, SourceURL: sourceURL, RateDate: date, QuotedAt: now, ExpiresAt: expires}, nil
}

func (s *Service) preparePaymentQuote(ctx context.Context, m Merchant, o Order) (Order, error) {
	if o.Money.Currency == o.Settlement().Currency || m.Driver == "evm_wallet" {
		return o, nil
	}
	// A manually agreed original amount remains a manual receipt fact; it is
	// not evidence that those funds already discharge a different currency.
	if m.Driver == "manual" && o.Money.Amount.Sign() > 0 {
		return o, nil
	}
	if m.Driver == "nowpayments" {
		currency := o.Settlement().Currency
		if currency != "USD" && currency != "EUR" && currency != "GBP" && currency != "JPY" {
			rate, err := s.referenceRate(ctx, currency, "USD", false)
			if err != nil {
				return o, err
			}
			invoice := o
			invoice.Money.Currency = "USD"
			q, err := quoteMoney(invoice, rate.Rate, "Frankfurter", "https://frankfurter.dev/", rate.RateDate)
			if err != nil {
				return o, err
			}
			invoiceMoney := q.PayMoney
			o.ProviderPriceMoney = &invoiceMoney
			o.PaymentQuote = &q
		}
		return o, nil
	}
	if _, fiat := fiatMinorDigits(o.Money.Currency); !fiat {
		return o, nil
	}
	// A channel-specific quote takes precedence over a general reference rate.
	var q PaymentQuote
	var err error
	if k, ok := s.Drivers[m.Driver].(PaymentQuoter); ok {
		q, err = k.QuotePayment(ctx, m, o)
	} else {
		err = ErrUnsupported
	}
	if err != nil {
		rate, e := s.referenceRate(ctx, o.Settlement().Currency, o.Money.Currency, false)
		if e != nil {
			return o, e
		}
		q, err = quoteMoney(o, rate.Rate, "Frankfurter", "https://frankfurter.dev/", rate.RateDate)
	}
	if err != nil {
		return o, err
	}
	if q.PayMoney.Validate() != nil || q.PayMoney.Currency != o.Money.Currency || q.SettlementMoney.Currency != o.Settlement().Currency || q.SettlementMoney.Amount.Cmp(o.Settlement().Amount) != 0 || q.ID == "" || q.Source == "" || !q.ExpiresAt.After(time.Now()) || q.ExpiresAt.After(o.ExpiresAt) {
		return o, ErrQuoteUnavailable
	}
	o.Money = q.PayMoney
	o.PaymentQuote = &q
	return o, nil
}
