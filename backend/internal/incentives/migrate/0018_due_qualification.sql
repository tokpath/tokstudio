CREATE INDEX incentives_pending_qualification ON incentives_confirmation_events(confirm_at, customer_id)
    INCLUDE (operator_id) WHERE status='pending';
CREATE INDEX incentives_due_frozen_commissions ON incentives_commission_entries(confirm_at, beneficiary_id)
    INCLUDE (operator_id, event_id) WHERE status='frozen';
CREATE INDEX incentives_unqualified_balance ON incentives_qualification_accounts(customer_id, operator_id)
    INCLUDE (confirmed_money) WHERE acquired_at IS NULL;
CREATE INDEX incentives_pending_offset ON incentives_qualification_accounts(customer_id, operator_id)
    WHERE pending_offset_money>0;
