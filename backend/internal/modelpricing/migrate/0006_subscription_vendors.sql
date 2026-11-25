ALTER TABLE model_actual_costs
 DROP CONSTRAINT model_actual_cost_subscription_statement,
 ADD CONSTRAINT model_actual_cost_subscription_statement CHECK (
   (subscription_kind='' AND statement_reference='' AND account_coverage='') OR
   (subscription_kind IN ('codex','claude','grok','gemini','antigravity') AND statement_reference<>'' AND supplier_model_id='')
 );
