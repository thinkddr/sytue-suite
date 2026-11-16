-- SPDX-License-Identifier: Apache-2.0
-- A restrictive guard never grants merge writes. The separate SELECT policy
-- exposes existence to the source/target preflight, never a write capability.
CREATE OR REPLACE FUNCTION sytue_scope_merge_allowed()
RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT current_setting('app.merge',true) IS DISTINCT FROM 'on'
$$;
CREATE OR REPLACE FUNCTION sytue_scope_merge_preflight(row_tenant text)
RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT current_setting('app.merge_preflight',true)='on'
 AND row_tenant IN (current_setting('app.merge_source',true),current_setting('app.merge_target',true))
$$;
CREATE OR REPLACE FUNCTION sytue_scope_merge_register(scope_table regclass)
RETURNS void LANGUAGE plpgsql AS $$
DECLARE qualified text;
BEGIN
 SELECT format('%I.%I',n.nspname,c.relname) INTO qualified
 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
 WHERE c.oid=scope_table AND n.nspname=current_schema() AND c.relkind IN('r','p')
 AND EXISTS(SELECT 1 FROM pg_attribute a WHERE a.attrelid=c.oid AND a.attname='tenant' AND a.attnum>0 AND NOT a.attisdropped);
 IF qualified IS NULL THEN RAISE EXCEPTION 'scope merge table invalid' USING ERRCODE='22023'; END IF;
 EXECUTE format('ALTER TABLE %s ENABLE ROW LEVEL SECURITY',qualified);
 EXECUTE format('ALTER TABLE %s FORCE ROW LEVEL SECURITY',qualified);
 EXECUTE format('ALTER TABLE %s ADD CONSTRAINT scope_bound_merge_contract CHECK (sytue_scope_merge_allowed())',qualified);
 EXECUTE format('CREATE POLICY scope_bound_merge_write ON %s AS RESTRICTIVE FOR ALL USING (sytue_scope_merge_allowed()) WITH CHECK (sytue_scope_merge_allowed())',qualified);
 EXECUTE format('CREATE POLICY scope_bound_merge_read ON %s FOR SELECT USING (sytue_scope_merge_preflight(tenant))',qualified);
END $$;
CREATE OR REPLACE FUNCTION sytue_scope_merge_unregister(scope_tables regclass[])
RETURNS void LANGUAGE plpgsql SET row_security=off AS $$
DECLARE scope_table regclass;present boolean;
BEGIN
 FOREACH scope_table IN ARRAY scope_tables LOOP
  EXECUTE format('LOCK TABLE %s IN ACCESS EXCLUSIVE MODE',scope_table);
 END LOOP;
 FOREACH scope_table IN ARRAY scope_tables LOOP
  EXECUTE format('SELECT EXISTS(SELECT 1 FROM %s)',scope_table) INTO present;
  IF present THEN RAISE EXCEPTION 'scope-bound merge protection is retained' USING ERRCODE='PST01'; END IF;
 END LOOP;
 FOREACH scope_table IN ARRAY scope_tables LOOP
  EXECUTE format('ALTER TABLE %s DROP CONSTRAINT scope_bound_merge_contract',scope_table);
  EXECUTE format('DROP POLICY scope_bound_merge_read ON %s',scope_table);
  EXECUTE format('DROP POLICY scope_bound_merge_write ON %s',scope_table);
 END LOOP;
END $$;
