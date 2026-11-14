package portal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/jobs"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
	"github.com/tokpath/tokstudio/backend/internal/operations"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type scheduledTask struct {
	ID, Kind             string
	Payload              json.RawMessage `gorm:"type:jsonb"`
	LastJobID            *string
	NextDueAt, CreatedAt time.Time
}

func (scheduledTask) TableName() string { return "portal_schedules" }

type workerPayload struct {
	OperatorID string `json:"operator_id,omitempty"`
	CustomerID string `json:"customer_id,omitempty"`
	ID         string `json:"id,omitempty"`
	Cursor     string `json:"cursor,omitempty"`
	Version    int64  `json:"version,omitempty"`
}

func (a *Application) schedule(ctx context.Context, kind, subject string, p workerPayload, interval time.Duration) error {
	raw, _ := json.Marshal(p)
	id := kind + ":" + subject
	return a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var now time.Time
		if e := tx.Raw("SELECT clock_timestamp()").Row().Scan(&now); e != nil {
			return e
		}
		s := scheduledTask{ID: id, Kind: kind, Payload: raw, NextDueAt: now, CreatedAt: now}
		if e := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&s).Error; e != nil {
			return e
		}
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&s, "id=?", id).Error; e != nil {
			return e
		}
		if now.Before(s.NextDueAt) {
			return nil
		}
		if s.LastJobID != nil {
			status, e := a.Jobs.StatusTx(tx, *s.LastJobID)
			if e != nil {
				return e
			}
			if status != "completed" && status != "dead" {
				return nil
			}
		}
		job, e := a.Jobs.EnqueueTx(tx, jobs.Input{Queue: "portal-maintenance", Kind: kind, BusinessKey: id + ":" + fmt.Sprint(s.NextDueAt.UnixNano()), Payload: raw, AvailableAt: now, MaxAttempts: 12})
		if e != nil {
			return e
		}
		return tx.Model(&scheduledTask{}).Where("id=?", id).Updates(map[string]any{"payload": raw, "last_job_id": job.ID, "next_due_at": now.Add(interval)}).Error
	})
}
func (a *Application) progress(ctx context.Context, id string) (string, error) {
	var row struct{ Cursor string }
	e := a.DB.WithContext(ctx).Table("portal_worker_progress").Where("id=?", id).Limit(1).Find(&row).Error
	return row.Cursor, e
}
func (a *Application) setProgress(ctx context.Context, id, cursor string) error {
	return a.DB.WithContext(ctx).Exec("INSERT INTO portal_worker_progress(id,cursor) VALUES(?,?) ON CONFLICT(id) DO UPDATE SET cursor=EXCLUDED.cursor", id, cursor).Error
}
func (a *Application) DiscoverWorkerTasks(ctx context.Context) error {
	for _, kind := range []string{"registration_release", "unpaid_orders", "kvoucher_payments", "incentive_events", "remediation_events", "catalog_changes", "retention"} {
		interval := time.Minute
		if kind == "kvoucher_payments" {
			interval = 5 * time.Second
		}
		if e := a.schedule(ctx, kind, "global", workerPayload{}, interval); e != nil {
			return e
		}
	}
	if e := a.queueCommerceNotifications(ctx); e != nil {
		return e
	}
	domains, domainErr := a.Branding.PendingDomainChecks(ctx)
	if domainErr != nil {
		return domainErr
	}
	for _, domain := range domains {
		if e := a.schedule(ctx, "brand_domain", domain.ID, workerPayload{ID: domain.ID}, 5*time.Minute); e != nil {
			return e
		}
	}
	cursor, e := a.progress(ctx, "operators")
	if e != nil {
		return e
	}
	operators, e := a.Organization.OperatorPage(ctx, nil, cursor, 100)
	if e != nil {
		return e
	}
	next := ""
	for _, op := range operators {
		for _, kind := range []string{"risks", "restriction_reviews", "invoice_review"} {
			if kind == "risks" && op.Kind == organization.Platform {
				continue
			}
			if e = a.schedule(ctx, kind, op.ID, workerPayload{OperatorID: op.ID}, time.Minute); e != nil {
				return e
			}
		}
		next = op.ID
	}
	if len(operators) < 100 {
		next = ""
	}
	if e = a.setProgress(ctx, "operators", next); e != nil {
		return e
	}
	cursor, e = a.progress(ctx, "customers")
	if e != nil {
		return e
	}
	customers, e := a.Incentives.DueQualificationPage(ctx, cursor, 100)
	if e != nil {
		return e
	}
	next = ""
	for _, customer := range customers {
		if e = a.schedule(ctx, "qualification", customer.OperatorID+":"+customer.CustomerID, workerPayload{OperatorID: customer.OperatorID, CustomerID: customer.CustomerID}, time.Minute); e != nil {
			return e
		}
		next = customer.CustomerID
	}
	if len(customers) < 100 {
		next = ""
	}
	if e = a.setProgress(ctx, "customers", next); e != nil {
		return e
	}
	cursor, e = a.progress(ctx, "expiry")
	if e != nil {
		return e
	}
	expiring, e := a.Ledger.ExpiringCustomerCreditsPage(ctx, nil, time.Now().UTC(), cursor, 100)
	if e != nil {
		return e
	}
	next = ""
	for _, customer := range expiring {
		if e = a.schedule(ctx, "expiry", customer.OperatorID+":"+customer.CustomerID, workerPayload{OperatorID: customer.OperatorID, CustomerID: customer.CustomerID}, time.Minute); e != nil {
			return e
		}
		next = customer.OperatorID + ":" + customer.CustomerID
	}
	if len(expiring) < 100 {
		next = ""
	}
	if e = a.setProgress(ctx, "expiry", next); e != nil {
		return e
	}
	cursor, e = a.progress(ctx, "renewals")
	if e != nil {
		return e
	}
	subscriptions, e := a.Commerce.DueRenewalsPage(ctx, nil, time.Now().UTC(), 72*time.Hour, cursor, 100)
	if e != nil {
		return e
	}
	next = ""
	for _, s := range subscriptions {
		if e = a.schedule(ctx, "renewal", s.ID, workerPayload{OperatorID: s.OperatorID, CustomerID: s.CustomerID, ID: s.ID}, time.Minute); e != nil {
			return e
		}
		next = s.ID
	}
	if len(subscriptions) < 100 {
		next = ""
	}
	if e = a.setProgress(ctx, "renewals", next); e != nil {
		return e
	}
	cursor, e = a.progress(ctx, "usage")
	if e != nil {
		return e
	}
	requests, e := a.Usage.PendingRequestsPage(ctx, cursor, 100)
	if e != nil {
		return e
	}
	next = ""
	for _, r := range requests {
		if e = a.schedule(ctx, "usage", r.ID, workerPayload{ID: r.ID, CustomerID: r.CustomerID}, 15*time.Second); e != nil {
			return e
		}
		next = r.ID
	}
	if len(requests) < 100 {
		next = ""
	}
	if e = a.setProgress(ctx, "usage", next); e != nil {
		return e
	}
	cursor, e = a.progress(ctx, "deliveries")
	if e != nil {
		return e
	}
	orders, e := a.Commerce.PendingDeliveriesPage(ctx, nil, cursor, 100)
	if e != nil {
		return e
	}
	next = ""
	for _, o := range orders {
		if e = a.schedule(ctx, "delivery", o.ID, workerPayload{ID: o.ID, CustomerID: o.CustomerID, OperatorID: o.OperatorID}, time.Minute); e != nil {
			return e
		}
		next = o.ID
	}
	if len(orders) < 100 {
		next = ""
	}
	if e = a.setProgress(ctx, "deliveries", next); e != nil {
		return e
	}
	cursor, e = a.progress(ctx, "registration_rewards")
	if e != nil {
		return e
	}
	promises, e := a.Incentives.PendingRegistrationRewardsPage(ctx, nil, cursor, 100)
	if e != nil {
		return e
	}
	next = ""
	for _, id := range promises {
		if e = a.schedule(ctx, "registration_reward", id, workerPayload{ID: id}, time.Minute); e != nil {
			return e
		}
		next = id
	}
	if len(promises) < 100 {
		next = ""
	}
	return a.setProgress(ctx, "registration_rewards", next)
}
func (a *Application) queueCommerceNotifications(ctx context.Context) error {
	return a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var events []struct{ ID, OperatorID, Kind string }
		if err := tx.Raw(`SELECT e.id,e.operator_id,e.kind FROM commerce_events e
 WHERE e.published_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM durable_jobs j WHERE j.queue='portal-commerce-notifications'
                   AND j.kind='commerce_notification' AND j.business_key=e.id::text)
 ORDER BY e.created_at,e.id LIMIT 100 FOR UPDATE OF e SKIP LOCKED`).Scan(&events).Error; err != nil {
			return err
		}
		var acknowledged []string
		for _, event := range events {
			if !operations.HasCommerceNotification(event.Kind) {
				acknowledged = append(acknowledged, event.ID)
				continue
			}
			payload, _ := json.Marshal(workerPayload{ID: event.ID, OperatorID: event.OperatorID})
			if _, err := a.Jobs.EnqueueTx(tx, jobs.Input{Queue: "portal-commerce-notifications", Kind: "commerce_notification", BusinessKey: event.ID, Payload: payload, MaxAttempts: 12}); err != nil {
				return err
			}
		}
		if len(acknowledged) != 0 {
			return tx.Table("commerce_events").Where("id IN ?", acknowledged).Update("published_at", gorm.Expr("clock_timestamp()")).Error
		}
		return nil
	})
}

func (a *Application) runMaintenance(ctx context.Context, job jobs.Job) error {
	var p workerPayload
	if e := json.Unmarshal(job.Payload, &p); e != nil {
		return e
	}
	switch job.Kind {
	case "brand_domain":
		return a.Branding.CheckPendingDomain(ctx, p.ID, a.DomainVerifier)
	case "catalog_changes":
		_, e := a.CatalogChanges.Run(ctx, 10)
		return e
	case "registration_release":
		_, e := a.Incentives.ReleaseExpiredRegistrationPromises(ctx, nil, 100)
		return e
	case "unpaid_orders":
		reconcileErr := a.reconcileExpiredPayments(ctx)
		return errors.Join(reconcileErr, a.expireUnpaidOrders(ctx, 50))
	case "kvoucher_payments":
		if err := a.Payments.RefreshQuoteRates(ctx); err != nil {
			return err
		}
		return a.reconcileOpenKVoucher(ctx, 10)
	case "registration_reward":
		return a.Incentives.DeliverRegistrationRewards(ctx, nil, p.ID)
	case "remediation_events":
		_, e := a.Remediation.RelayEvents(ctx, nil, 100, a.publishRemediationEvent)
		return e
	case "incentive_events":
		_, e := a.Incentives.RelayEvents(ctx, nil, 100, operations.IncentiveNotifications{Operations: a.Operations, Audience: a.Incentives})
		return e
	case "commerce_notification":
		return a.Commerce.PublishEvent(ctx, nil, p.ID, operations.CommerceNotifications{Operations: a.Operations, Audience: a.Commerce})
	case "qualification":
		return a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			own, e := a.Organization.CurrentOwnership(ctx, tx, p.CustomerID)
			if e != nil {
				return e
			}
			if own.OperatorID != p.OperatorID {
				return nil
			}
			var at time.Time
			if e = tx.Raw("SELECT clock_timestamp()").Row().Scan(&at); e != nil {
				return e
			}
			_, e = a.Incentives.Advance(ctx, tx, p.OperatorID, p.CustomerID, at)
			return e
		})
	case "expiry":
		return a.Usage.ExpireCustomerCredits(ctx, p.OperatorID, p.CustomerID)
	case "renewal":
		return a.runRenewal(ctx, p)
	case "renewal_pause":
		return a.runRenewalPause(ctx, p)
	case "package_change":
		return a.runPackageChange(ctx, p)
	case "usage":
		r, e := a.Usage.GetRequest(ctx, p.CustomerID, p.ID)
		if e != nil {
			return e
		}
		if r.Status == "completed" && r.Operation == modelruntime.Video {
			_, e = a.Usage.RefreshAsync(ctx, r.ID)
			return e
		}
		if !time.Now().Before(r.UsageDeadline) {
			_, e = a.Usage.ResolveTimeout(ctx, r.ID)
			return e
		}
		if r.Operation == modelruntime.Video {
			_, e = a.Usage.RefreshAsync(ctx, r.ID)
			return e
		}
		return nil
	case "delivery":
		o, e := a.Commerce.GetOrder(ctx, nil, p.CustomerID, p.ID)
		if e != nil {
			return e
		}
		return a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			return a.fulfillCustomerOrder(ctx, tx, o, "system:delivery-recovery")
		})
	case "risks":
		_, e := a.Operations.RunRisks(ctx, p.OperatorID)
		return e
	case "restriction_reviews":
		_, e := a.Operations.RunRestrictionReviews(ctx, p.OperatorID)
		return e
	case "invoice_review":
		_, e := a.Operations.ReviewInvoices(ctx, p.OperatorID, 50)
		return e
	case "retention":
		_, resultsErr := a.Operations.RunRetention(ctx, time.Now().UTC(), 100)
		accountErr := a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if e := tx.Model(&queuedMessage{}).Where("expires_at<=? AND ciphertext<>''", time.Now()).Updates(map[string]any{"ciphertext": "", "status": "expired"}).Error; e != nil {
				return e
			}
			return tx.Where("expires_at<=?", time.Now()).Delete(&browserSession{}).Error
		})
		return errors.Join(resultsErr, accountErr)
	}
	return errors.New("unknown maintenance kind")
}
func safeWorkerError(kind string, e error) string {
	switch {
	case errors.Is(e, organization.ErrBusinessClosed):
		return kind + ": business_closed"
	case errors.Is(e, ledger.ErrInsufficient):
		return kind + ": insufficient_asset"
	case errors.Is(e, commerce.ErrNotPaid):
		return kind + ": original_order_not_fully_paid"
	case errors.Is(e, organization.ErrStaleOwnership):
		return kind + ": ownership_changed"
	}
	return kind + ": dependency_or_state_requires_review"
}
func (a *Application) RunMaintenanceJobs(ctx context.Context, owner string, limit int) error {
	return a.runPortalJobs(ctx, "portal-maintenance", owner, limit)
}

func (a *Application) runPortalJobs(ctx context.Context, queue, owner string, limit int) error {
	if limit < 1 || limit > 100 {
		limit = 1
	}
	for i := 0; i < limit; i++ {
		claimed, e := a.Jobs.Claim(ctx, queue, owner, 60*time.Second, 1)
		if e != nil {
			return e
		}
		if len(claimed) == 0 {
			return nil
		}
		job := claimed[0]
		taskCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
		e = a.runMaintenance(taskCtx, job)
		cancel()
		a.Metrics.Job(job.Kind, e != nil)
		if e != nil {
			if failure := a.Jobs.Fail(ctx, job.ID, owner, job.Fence, safeWorkerError(job.Kind, e)); failure != nil && !errors.Is(failure, jobs.ErrLeaseLost) {
				return failure
			}
			continue
		}
		if e = a.Jobs.Complete(ctx, job.ID, owner, job.Fence); e != nil && !errors.Is(e, jobs.ErrLeaseLost) {
			return e
		}
	}
	return nil
}

// RunWorker runs the new application's queues. External I/O is performed only
// by APIs with their own durable intent/claim or by the currently leased job.
func (a *Application) RunWorker(ctx context.Context, concurrency int, onError func(string, error)) {
	if concurrency < 1 {
		concurrency = 4
	}
	if concurrency > 64 {
		concurrency = 64
	}
	report := func(kind string, e error) {
		if e != nil && ctx.Err() == nil && onError != nil {
			onError(kind, e)
		}
	}
	var workers sync.WaitGroup
	if a.ModelPricing != nil {
		workers.Add(1)
		go func() {
			defer workers.Done()
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			for {
				report("model_pricing", a.ModelPricing.RunSync(ctx))
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
			}
		}()
	}

	for i := 0; i < concurrency; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			owner := "worker:" + uuid.NewString()
			for {
				if ctx.Err() != nil {
					return
				}
				report("mail", a.RunMailJobs(ctx, owner, 1))
				report("delivery", a.RunDeliveryJobs(ctx, owner, 1))
				report("price_checks", a.RunPriceChecks(ctx, owner, 1))
				report("maintenance", a.RunMaintenanceJobs(ctx, owner, 1))
				report("commerce_notification", a.runPortalJobs(ctx, "portal-commerce-notifications", owner, 1))
				if _, e := a.Payments.RunPending(ctx, 1); e != nil {
					report("payments", e)
				}
				if _, e := a.Operations.RunNotifications(ctx, 1); e != nil {
					report("notifications", e)
				}
				if _, e := a.Operations.RunExports(ctx, 1); e != nil {
					report("exports", e)
				}
				if _, e := a.AutoReplenishments.Run(ctx, 1); e != nil {
					report("replenishment", e)
				}
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Second):
				}
			}
		}()
	}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		e := a.DiscoverWorkerTasks(ctx)
		report("discovery", e)
		if e == nil {
			a.workerHeartbeat.Store(time.Now().UnixNano())
		}
		select {
		case <-ctx.Done():
			workers.Wait()
			return
		case <-ticker.C:
		}
	}
}
