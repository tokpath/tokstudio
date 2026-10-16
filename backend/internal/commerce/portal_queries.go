package commerce

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// PaymentOrderCustomer is a trusted settlement projection. Current and original
// responsibility must match; the caller never accepts a customer from a webhook.
func (s *Service) PaymentOrderCustomer(ctx context.Context, tx *gorm.DB, sourceOperator, orderID string) (string, error) {
	if tx == nil || !validID(sourceOperator) || !validID(orderID) {
		return "", ErrInvalidInput
	}
	var r orderRow
	if e := tx.WithContext(ctx).Select("customer_id").Where("id=? AND (original_operator_id=? OR operator_id=?)", orderID, sourceOperator, sourceOperator).Take(&r).Error; e != nil {
		return "", e
	}
	return r.CustomerID, nil
}

type OrderCursor struct {
	At time.Time `json:"at"`
	ID string    `json:"id"`
}

func NextOrderCursor(order Order) string {
	body, _ := json.Marshal(OrderCursor{order.CreatedAt, order.ID})
	return base64.RawURLEncoding.EncodeToString(body)
}
func cursorQuery(q *gorm.DB, cursor string) (*gorm.DB, error) {
	if cursor == "" {
		return q, nil
	}
	body, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return nil, ErrInvalidInput
	}
	var value OrderCursor
	if json.Unmarshal(body, &value) != nil || !validID(value.ID) || value.At.IsZero() {
		return nil, ErrInvalidInput
	}
	return q.Where("(created_at,id)<(?,?)", value.At, value.ID), nil
}
func (s *Service) ListCustomerOrdersPage(ctx context.Context, tx *gorm.DB, customer, cursor, query string, limit int) ([]Order, error) {
	if !validID(customer) || limit < 1 || limit > 100 {
		return nil, ErrInvalidInput
	}
	q := s.dbFor(ctx, tx).Where("customer_id=?", customer)
	return orderPage(q, cursor, query, limit)
}

// ListOperatorOrders never includes a child's retail orders. The application's
// staff permission check must authorize this exact operator before calling it.
func (s *Service) ListOperatorOrders(ctx context.Context, tx *gorm.DB, operator, cursor, query string, limit int) ([]Order, error) {
	if !validID(operator) || limit < 1 || limit > 100 {
		return nil, ErrInvalidInput
	}
	return orderPage(s.dbFor(ctx, tx).Where("operator_id=?", operator), cursor, query, limit)
}
func orderPage(q *gorm.DB, cursor, query string, limit int) ([]Order, error) {
	q, err := cursorQuery(q, cursor)
	if err != nil {
		return nil, err
	}
	if query != "" {
		q = q.Where("(id::text ILIKE ? OR kind ILIKE ? OR status ILIKE ?)", "%"+query+"%", "%"+query+"%", "%"+query+"%")
	}
	var rows []orderRow
	if err := q.Order("created_at DESC,id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]Order, len(rows))
	for i, row := range rows {
		result[i] = Order(row)
	}
	return result, nil
}

type DeliveredPackage struct {
	Order        Order
	Delivery     Delivery
	Subscription *Subscription
	Name         string
}

// ListCustomerDeliveries includes packages without auto-renewal. Five bounded
// queries load a complete page; delivery history is not confused with renewal
// authorizations and no per-order query is required.
func (s *Service) ListCustomerDeliveries(ctx context.Context, tx *gorm.DB, customer string, limit int) ([]DeliveredPackage, error) {
	if !validID(customer) || limit < 1 || limit > 100 {
		return nil, ErrInvalidInput
	}
	gdb := s.dbFor(ctx, tx)
	var orders []orderRow
	if err := gdb.Where("customer_id=? AND kind IN ('package','renewal') AND status='completed'", customer).Order("created_at DESC,id DESC").Limit(limit).Find(&orders).Error; err != nil {
		return nil, err
	}
	result := make([]DeliveredPackage, 0, len(orders))
	if len(orders) == 0 {
		return result, nil
	}
	orderIDs := make([]string, 0, len(orders))
	packageIDs := []string{}
	var quotes []quoteRow
	quoteIDs := []string{}
	for _, order := range orders {
		orderIDs = append(orderIDs, order.ID)
		if order.QuoteID != nil {
			quoteIDs = append(quoteIDs, *order.QuoteID)
		}
	}
	if err := gdb.Where("id IN ?", quoteIDs).Find(&quotes).Error; err != nil {
		return nil, err
	}
	quotePackages := map[string]string{}
	for _, quote := range quotes {
		quotePackages[quote.ID] = quote.PackageID
		packageIDs = append(packageIDs, quote.PackageID)
	}
	var packages []packageRow
	if err := gdb.Where("id IN ?", packageIDs).Find(&packages).Error; err != nil {
		return nil, err
	}
	names := map[string]string{}
	for _, pkg := range packages {
		names[pkg.ID] = pkg.Name
	}
	var deliveries []deliveryRow
	if err := gdb.Where("order_id IN ?", orderIDs).Find(&deliveries).Error; err != nil {
		return nil, err
	}
	byOrder := map[string]Delivery{}
	for _, delivery := range deliveries {
		byOrder[delivery.OrderID] = Delivery(delivery)
	}
	var subscriptions []subscriptionRow
	if err := gdb.Where("customer_id=? AND source_order_id IN ?", customer, orderIDs).Find(&subscriptions).Error; err != nil {
		return nil, err
	}
	bySubscription := map[string]Subscription{}
	for _, subscription := range subscriptions {
		bySubscription[subscription.SourceOrderID] = Subscription(subscription)
	}
	for _, row := range orders {
		item := DeliveredPackage{Order: Order(row), Delivery: byOrder[row.ID]}
		if row.QuoteID != nil {
			item.Name = names[quotePackages[*row.QuoteID]]
		}
		if subscription, ok := bySubscription[row.ID]; ok {
			copy := subscription
			item.Subscription = &copy
		}
		result = append(result, item)
	}
	return result, nil
}
func (s *Service) OperatorReceipts(ctx context.Context, tx *gorm.DB, operator, cursor string, limit int) ([]Receipt, error) {
	if !validID(operator) || limit < 1 || limit > 100 || (cursor != "" && !validID(cursor)) {
		return nil, ErrInvalidInput
	}
	q := s.dbFor(ctx, tx).Where("operator_id=?", operator).Order("id").Limit(limit)
	if cursor != "" {
		q = q.Where("id>?", cursor)
	}
	var rows []receiptRow
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]Receipt, len(rows))
	for i, row := range rows {
		result[i] = Receipt(row)
	}
	return result, nil
}

// CustomerInvoiceReceiptReferences returns only original receipts actually
// allocated to this customer's orders in the authenticated current scope.
func (s *Service) CustomerInvoiceReceiptReferences(ctx context.Context, tx *gorm.DB, operator, customer, q, cursor string, limit int) ([]Receipt, error) {
	if !validID(operator) || !validID(customer) || limit < 1 || limit > 50 || len(q) > 200 {
		return nil, ErrInvalidInput
	}
	rows := []Receipt{}
	query := s.dbFor(ctx, tx).Table("commerce_receipts r").Select("DISTINCT r.*").Joins("JOIN commerce_receipt_allocations allocation ON allocation.receipt_id=r.id").Joins("JOIN commerce_orders o ON o.id=allocation.order_id").Where("o.customer_id=? AND o.operator_id=?", customer, operator)
	if cursor != "" {
		if !validID(cursor) {
			return nil, ErrInvalidInput
		}
		query = query.Where("r.id::text>?", cursor)
	}
	if q != "" {
		pattern := "%" + q + "%"
		query = query.Where("(r.id::text ILIKE ? OR r.provider_receipt_id ILIKE ?)", pattern, pattern)
	}
	e := query.Order("r.id").Limit(limit).Scan(&rows).Error
	return rows, e
}

// CustomerHistoricalReceiptReferences returns only original receipts actually
// allocated to the authenticated stable customer identity, including original issuers.
func (s *Service) CustomerHistoricalReceiptReferences(ctx context.Context, tx *gorm.DB, customer, q, cursor string, limit int) ([]Receipt, error) {
	if !validID(customer) || limit < 1 || limit > 50 || len(q) > 200 {
		return nil, ErrInvalidInput
	}
	rows := []Receipt{}
	query := s.dbFor(ctx, tx).Table("commerce_receipts r").Select("DISTINCT r.*").Joins("JOIN commerce_receipt_allocations allocation ON allocation.receipt_id=r.id").Joins("JOIN commerce_orders o ON o.id=allocation.order_id").Where("o.customer_id=?", customer)
	if cursor != "" {
		if !validID(cursor) {
			return nil, ErrInvalidInput
		}
		query = query.Where("r.id::text>?", cursor)
	}
	if q != "" {
		pattern := "%" + q + "%"
		query = query.Where("(r.id::text ILIKE ? OR r.provider_receipt_id ILIKE ?)", pattern, pattern)
	}
	e := query.Order("r.id").Limit(limit).Scan(&rows).Error
	return rows, e
}
func (s *Service) SearchOperatorReceipts(ctx context.Context, tx *gorm.DB, operator, q, cursor string, limit int) ([]Receipt, error) {
	if !validID(operator) || limit < 1 || limit > 50 || len(q) > 200 {
		return nil, ErrInvalidInput
	}
	rows := []Receipt{}
	query := s.dbFor(ctx, tx).Table("commerce_receipts").Where("operator_id=?", operator)
	if cursor != "" {
		if !validID(cursor) {
			return nil, ErrInvalidInput
		}
		query = query.Where("id::text>?", cursor)
	}
	if q != "" {
		pattern := "%" + q + "%"
		query = query.Where("(id::text ILIKE ? OR provider_receipt_id ILIKE ? OR payer_reference ILIKE ?)", pattern, pattern, pattern)
	}
	e := query.Order("id").Limit(limit).Scan(&rows).Error
	return rows, e
}
