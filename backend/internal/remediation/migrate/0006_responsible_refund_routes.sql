ALTER TABLE remediation_refund_destination_proposals ADD COLUMN responsible_execution boolean NOT NULL DEFAULT false;
ALTER TABLE remediation_refund_destination_authorizations ADD COLUMN responsible_execution boolean NOT NULL DEFAULT false;
ALTER TABLE remediation_refund_destination_authorizations ADD COLUMN route_exception_reference text NOT NULL DEFAULT '';
