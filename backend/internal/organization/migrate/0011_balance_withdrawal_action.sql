-- Cash-out is an account withdrawal, not a refund of a top-up receipt.
UPDATE organization_scope_freezes SET action='balance_withdrawal' WHERE action='wallet_refund';
