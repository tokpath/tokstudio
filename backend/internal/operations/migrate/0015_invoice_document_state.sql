-- A review of an unissued request must leave it available for rejection.
UPDATE operations_invoices SET state='requested'
WHERE state='review_required' AND invoice_reference='';
