ALTER TABLE cooperation_procurement_settlements
 RENAME COLUMN platform_shortfall TO shortfall;

ALTER TABLE cooperation_procurement_settlements
 DROP CONSTRAINT cooperation_procurement_settlements_buyer_id_fkey,
 ADD FOREIGN KEY (buyer_id) REFERENCES organization_operators(id),
 ALTER COLUMN hold_id DROP NOT NULL,
 ADD COLUMN supplier_id UUID REFERENCES organization_operators(id),
 ADD COLUMN shortfall_operator_id UUID REFERENCES organization_operators(id);

-- Settlements written under the previous policy already assigned shortfalls
-- to the platform. Keep their amounts and responsibility as originally booked.
UPDATE cooperation_procurement_settlements s
 SET supplier_id=p.supplier_id,
     shortfall_operator_id=CASE WHEN s.shortfall>0 THEN
      (SELECT id FROM organization_operators WHERE kind='platform')
     END
 FROM cooperation_procurement_positions p
 WHERE s.buyer_id=p.buyer_id;

ALTER TABLE cooperation_procurement_settlements
 ALTER COLUMN supplier_id SET NOT NULL,
 ADD CHECK (buyer_id<>supplier_id AND buyer_id IS DISTINCT FROM shortfall_operator_id),
 ADD CHECK ((shortfall>0)=(shortfall_operator_id IS NOT NULL));

CREATE INDEX cooperation_procurement_shortfall_operator_time
 ON cooperation_procurement_settlements(shortfall_operator_id,created_at)
 WHERE shortfall_operator_id IS NOT NULL;
