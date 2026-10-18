ALTER TABLE operations_risk_rules ADD COLUMN name TEXT NOT NULL DEFAULT '';

UPDATE operations_risk_rules SET name = CASE metric
 WHEN 'sales_space_atg' THEN '剩余销售空间 ATG预警'
 WHEN 'procurement_available_atg' THEN '采购可用余额 ATG预警'
 WHEN 'oversell_used_atg' THEN '已用超卖额度 ATG预警'
 WHEN 'pending_usage' THEN '在途及待结算请求数预警'
 WHEN 'unverified_costs' THEN '待核实上游成本数预警'
 ELSE metric || '预警' END;

ALTER TABLE operations_risk_rules ADD CONSTRAINT operations_risk_rule_name_length CHECK (length(btrim(name)) BETWEEN 1 AND 100);
