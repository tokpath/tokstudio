INSERT INTO payment_method_versions(id,method_id,operator_id,version,name,collection)
SELECT gen_random_uuid(),id,operator_id,1,driver,true FROM payments_merchants;
