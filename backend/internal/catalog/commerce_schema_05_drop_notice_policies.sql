-- Already-applied migration files stay unchanged. This later step removes the
-- unused per-operator notice policy table; the lead time now lives in code.
DROP TABLE IF EXISTS tokenhub_catalog_notice_policies;
