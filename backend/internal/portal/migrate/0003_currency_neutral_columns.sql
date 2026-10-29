-- A structural rename only. Native ISO currency values and all amounts stay intact.
CREATE OR REPLACE FUNCTION ledger_neutral_money_json(value JSONB) RETURNS JSONB LANGUAGE sql IMMUTABLE AS $$
 SELECT CASE jsonb_typeof(value)
 WHEN 'object' THEN COALESCE((SELECT jsonb_object_agg(replace(replace(replace(key,'_usd','_money'),'usd_','money_'),'USD','Money'),CASE WHEN key IN ('evidence','raw','raw_body','provider_response','external_response') THEN val ELSE ledger_neutral_money_json(val) END) FROM jsonb_each(value) AS x(key,val)),'{}'::jsonb)
 WHEN 'array' THEN COALESCE((SELECT jsonb_agg(ledger_neutral_money_json(val) ORDER BY ord) FROM jsonb_array_elements(value) WITH ORDINALITY x(val,ord)),'[]'::jsonb)
 ELSE value END
$$;
DO $migration$
DECLARE c RECORD; t RECORD; f RECORD; definition TEXT;
BEGIN
 FOR c IN SELECT table_name,column_name FROM information_schema.columns WHERE table_schema=current_schema() AND left(table_name,7)='portal_' AND (column_name LIKE '%\_usd%' ESCAPE '\' OR column_name LIKE 'usd\_%' ESCAPE '\') LOOP
  EXECUTE format('ALTER TABLE %I RENAME COLUMN %I TO %I',c.table_name,c.column_name,replace(replace(c.column_name,'_usd','_money'),'usd_','money_'));
 END LOOP;
 FOR t IN SELECT DISTINCT table_name FROM information_schema.columns WHERE table_schema=current_schema() AND left(table_name,7)='portal_' AND data_type='jsonb' LOOP
  EXECUTE format('ALTER TABLE %I DISABLE TRIGGER USER',t.table_name);
  FOR c IN SELECT column_name FROM information_schema.columns WHERE table_schema=current_schema() AND table_name=t.table_name AND data_type='jsonb' LOOP
   EXECUTE format('UPDATE %I SET %I=ledger_neutral_money_json(%I) WHERE %I IS NOT NULL',t.table_name,c.column_name,c.column_name,c.column_name);
  END LOOP;
  EXECUTE format('ALTER TABLE %I ENABLE TRIGGER USER',t.table_name);
 END LOOP;
 -- PL/pgSQL trigger text is not rewritten by ALTER COLUMN RENAME.
 FOR f IN SELECT p.oid FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname=current_schema() AND p.prokind='f' AND p.proname<>'ledger_neutral_money_json' AND left(p.proname,7)='portal_' LOOP
  definition:=pg_get_functiondef(f.oid);
  IF position('_usd' in definition)>0 THEN EXECUTE replace(definition,'_usd','_money'); END IF;
 END LOOP;
END $migration$;
