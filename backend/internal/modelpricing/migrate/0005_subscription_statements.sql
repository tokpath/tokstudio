ALTER TABLE model_actual_costs
 ADD COLUMN subscription_kind text NOT NULL DEFAULT '',
 ADD COLUMN statement_reference text NOT NULL DEFAULT '',
 ADD COLUMN account_coverage text NOT NULL DEFAULT '',
 ADD CONSTRAINT model_actual_cost_subscription_statement CHECK (
   (subscription_kind='' AND statement_reference='' AND account_coverage='') OR
   (subscription_kind IN ('codex','claude') AND statement_reference<>'' AND supplier_model_id='')
 );
CREATE UNIQUE INDEX model_actual_cost_unique_subscription_statement
 ON model_actual_costs(subscription_kind, statement_reference) WHERE subscription_kind<>'';
