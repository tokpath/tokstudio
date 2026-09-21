-- Read-only exception report. Amounts are micro-USD (1 USD = 1,000,000).
-- Run with psql -v ON_ERROR_STOP=1 -f scripts/prelaunch_reconciliation.sql.
-- No inferred correction is posted. Review legacy/imported opening balances separately.
BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT w.id AS wallet_id, u.email,
 w.available_minor+w.reserved_minor AS api_balance,
 COALESCE(SUM(l.amount_minor) FILTER (WHERE l.event_type NOT IN ('authorization','release','commission_credit','commission_debit','commission_payout')),0) AS api_ledger_net,
 w.commission_available_minor AS commission_balance,
 COALESCE(SUM(l.amount_minor) FILTER (WHERE l.event_type IN ('commission_credit','commission_debit','commission_payout')),0) AS commission_ledger_net
FROM billing_wallets w LEFT JOIN identity_users u ON u.id=w.user_id LEFT JOIN billing_ledger l ON l.wallet_id=w.id
GROUP BY w.id,u.email
HAVING w.available_minor+w.reserved_minor <> COALESCE(SUM(l.amount_minor) FILTER (WHERE l.event_type NOT IN ('authorization','release','commission_credit','commission_debit','commission_payout')),0)
 OR w.commission_available_minor <> COALESCE(SUM(l.amount_minor) FILTER (WHERE l.event_type IN ('commission_credit','commission_debit','commission_payout')),0);

-- Paid settlements without a matching wallet payout require original payment evidence.
SELECT s.id AS settlement_id,s.amount_minor,s.status,p.reference AS payment_reference,
 COALESCE((SELECT -SUM(l.amount_minor) FROM billing_ledger l WHERE l.event_type='commission_payout' AND l.reference_id=s.id),0) AS wallet_payout_minor
FROM commission_settlements s LEFT JOIN commission_payouts p ON p.settlement_id=s.id
WHERE p.status='paid' OR s.status='paid'
ORDER BY s.created_at;

-- Older active commissions cannot be recalculated safely without their original snapshot.
SELECT status,COUNT(*) AS records_without_policy_snapshot,SUM(amount_minor) AS amount_minor
FROM commission_entries WHERE policy_snapshot IS NULL AND status<>'reversed' GROUP BY status;
COMMIT;
