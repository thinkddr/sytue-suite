-- SPDX-License-Identifier: Apache-2.0
-- Immutable v1 of «soltar la fuente» (ADR 0086). Domain migrations call
-- sytue_private_storage_release_install(<states>) once per state table.
--
-- v1 of the shared state guard (private_storage_v1.sql) never lets a frozen or
-- private source go back to legacy. This version keeps every v1 rule and adds
-- exactly one way back: sytue_private_storage_release, which the owner calls
-- with the epoch it saw. The domain rows were never touched while frozen (the
-- writer guard rejects them), so legacy again means the server data as it was
-- at the freeze. What was written later lives only on the devices.
--
-- The transaction-local GUC is not an authorization boundary (the application
-- role could set it, as it could set app.maintenance). It binds the transition
-- to one (table, tenant, owner, epoch) so no generic maintenance, merge or
-- domain UPDATE can reopen a source by accident. Authorization (owner, recent
-- reauthentication, explicit confirmation) is the service's job before calling.
CREATE OR REPLACE FUNCTION sytue_private_storage_release_key(state_table text,t text,o text,epoch uuid)
RETURNS text LANGUAGE sql IMMUTABLE SET search_path=pg_catalog,public AS $$
 SELECT jsonb_build_array('sytue-private-storage-release-v1',state_table,t,o,epoch::text)::text
$$;
CREATE OR REPLACE FUNCTION sytue_private_storage_state_scope_guard_v2()
RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,public AS $$
DECLARE released boolean:=false; before jsonb; after jsonb;
BEGIN
 before:=coalesce(to_jsonb(OLD)->'releases','[]'::jsonb);
 IF TG_OP='UPDATE' THEN after:=coalesce(to_jsonb(NEW)->'releases','[]'::jsonb); END IF;
 IF TG_OP='UPDATE' AND OLD.mode IN ('frozen','private') AND NEW.mode='legacy'
    AND OLD.epoch IS NOT NULL
    AND coalesce(current_setting('app.private_storage_release',true),'')=
        public.sytue_private_storage_release_key(TG_TABLE_NAME,OLD.tenant,OLD.owner_sub,OLD.epoch) THEN
  -- The only reopening: same row identity, a clean legacy state and exactly one
  -- audit entry appended for this epoch.
  IF NEW.tenant IS DISTINCT FROM OLD.tenant OR NEW.owner_sub IS DISTINCT FROM OLD.owner_sub
     OR NEW.epoch IS NOT NULL OR NEW.started_at IS NOT NULL
     OR NEW.source_rows<>0 OR NEW.source_blobs<>0
     OR (to_jsonb(NEW)->'account_id') IS DISTINCT FROM (to_jsonb(OLD)->'account_id')
     OR jsonb_typeof(after)<>'array' OR jsonb_array_length(after)<>jsonb_array_length(before)+1
     OR after-(jsonb_array_length(after)-1)<>before
     OR (after->-1->>'epoch') IS DISTINCT FROM OLD.epoch::text THEN
   RAISE EXCEPTION USING ERRCODE='PST01',MESSAGE='private_storage_scope_immutable';
  END IF;
  released:=true;
 END IF;
 IF NOT released THEN
  -- A legacy row may still be deleted as in v1 (an account erasure takes its
  -- audit with it); while it lives, its audit only grows through a release.
  IF TG_OP='UPDATE' AND after IS DISTINCT FROM before THEN
   RAISE EXCEPTION USING ERRCODE='PST01',MESSAGE='private_storage_scope_immutable';
  END IF;
  IF OLD.mode<>'legacy' THEN
   IF TG_OP='DELETE' OR NEW.tenant IS DISTINCT FROM OLD.tenant
    OR NEW.owner_sub IS DISTINCT FROM OLD.owner_sub OR NEW.epoch IS DISTINCT FROM OLD.epoch
    OR NEW.started_at IS DISTINCT FROM OLD.started_at OR NEW.mode='legacy'
    OR (to_jsonb(NEW)->'account_id') IS DISTINCT FROM (to_jsonb(OLD)->'account_id')
    OR (OLD.mode='building' AND NEW.mode NOT IN ('building','frozen'))
    OR (OLD.mode='frozen' AND NEW.mode NOT IN ('frozen','private'))
    OR (OLD.mode='private' AND NEW.mode<>'private')
    OR (OLD.mode IN ('frozen','private') AND
        (NEW.source_rows IS DISTINCT FROM OLD.source_rows OR NEW.source_blobs IS DISTINCT FROM OLD.source_blobs)) THEN
    RAISE EXCEPTION USING ERRCODE='PST01',MESSAGE='private_storage_scope_immutable';
   END IF;
  END IF;
 END IF;
 IF TG_OP='DELETE' THEN RETURN OLD; END IF;
 RETURN NEW;
END;
$$;
-- Owner context only (RLS by app.tenant/app.owner_sub); never under maintenance
-- or merge. Returns the new state; raises PST03 when the source is not frozen or
-- private at exactly `expected`, so a stale screen can never release a newer epoch.
CREATE OR REPLACE FUNCTION sytue_private_storage_release(state_table regclass,t text,o text,expected uuid,auth_time timestamptz,amr text[])
RETURNS TABLE(mode text,epoch uuid,started_at timestamptz,source_rows bigint,source_blobs bigint)
LANGUAGE plpgsql SET search_path=pg_catalog,public AS $$
DECLARE qualified text; relation text; old_mode text; old_epoch uuid; old_started timestamptz;
 old_rows bigint; old_blobs bigint; entry jsonb;
BEGIN
 qualified:=public.sytue_private_storage_table(state_table);
 IF t IS NULL OR t !~ '^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$'
    OR o IS NULL OR o='' OR octet_length(o)>1024 OR expected IS NULL OR auth_time IS NULL THEN
  RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='private_storage_scope_invalid';
 END IF;
 IF coalesce(current_setting('app.maintenance',true),'')='on' OR coalesce(current_setting('app.merge',true),'')='on'
    OR coalesce(current_setting('app.tenant',true),'')<>t THEN
  RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='private_storage_scope_invalid';
 END IF;
 SELECT c.relname INTO relation FROM pg_class c WHERE c.oid=state_table;
 EXECUTE format('SELECT s.mode,s.epoch,s.started_at,s.source_rows,s.source_blobs FROM %s s WHERE s.tenant=$1 AND s.owner_sub=$2 FOR UPDATE',qualified)
  INTO old_mode,old_epoch,old_started,old_rows,old_blobs USING t,o;
 IF old_mode IS NULL OR old_mode NOT IN ('frozen','private') OR old_epoch IS DISTINCT FROM expected THEN
  RAISE EXCEPTION USING ERRCODE='PST03',MESSAGE='private_storage_release_stale';
 END IF;
 entry:=jsonb_build_object('epoch',old_epoch::text,'mode',old_mode,'started_at',old_started,
  'source_rows',old_rows,'source_blobs',old_blobs,'released_at',clock_timestamp(),
  'auth_time',auth_time,'amr',to_jsonb(coalesce(amr,ARRAY[]::text[])));
 PERFORM set_config('app.private_storage_release',public.sytue_private_storage_release_key(relation,t,o,old_epoch),true);
 EXECUTE format('UPDATE %s SET mode=''legacy'',epoch=NULL,started_at=NULL,source_rows=0,source_blobs=0,
   releases=releases||jsonb_build_array($3::jsonb) WHERE tenant=$1 AND owner_sub=$2',qualified) USING t,o,entry;
 PERFORM set_config('app.private_storage_release','',true);
 RETURN QUERY SELECT 'legacy'::text,NULL::uuid,NULL::timestamptz,0::bigint,0::bigint;
END;
$$;
-- Swap every v1 state guard of one table for v2 (same trigger name, always
-- enabled) and add its append-only audit column. Fails if there is none.
CREATE OR REPLACE FUNCTION sytue_private_storage_release_install(state_table regclass)
RETURNS void LANGUAGE plpgsql SET search_path=pg_catalog,public AS $$
DECLARE qualified text; guard record; swapped int:=0;
BEGIN
 qualified:=public.sytue_private_storage_table(state_table);
 EXECUTE format('ALTER TABLE %s ADD COLUMN IF NOT EXISTS releases jsonb NOT NULL DEFAULT ''[]''::jsonb
   CONSTRAINT private_storage_releases_array CHECK(jsonb_typeof(releases)=''array'' AND jsonb_array_length(releases)<=256)',qualified);
 FOR guard IN SELECT g.tgname FROM pg_trigger g
   WHERE g.tgrelid=state_table AND NOT g.tgisinternal
     AND g.tgfoid IN ('public.sytue_private_storage_state_scope_guard()'::regprocedure,'public.sytue_private_storage_state_scope_guard_v2()'::regprocedure)
 LOOP
  EXECUTE format('DROP TRIGGER %I ON %s',guard.tgname,qualified);
  EXECUTE format('CREATE TRIGGER %I BEFORE UPDATE OR DELETE ON %s FOR EACH ROW EXECUTE FUNCTION public.sytue_private_storage_state_scope_guard_v2()',guard.tgname,qualified);
  EXECUTE format('ALTER TABLE %s ENABLE ALWAYS TRIGGER %I',qualified,guard.tgname);
  swapped:=swapped+1;
 END LOOP;
 IF swapped=0 THEN
  RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='private_storage_release_guard_missing';
 END IF;
END;
$$;
-- Down: back to the v1 guard. A released row is legacy and valid under v1.
CREATE OR REPLACE FUNCTION sytue_private_storage_release_uninstall(state_table regclass)
RETURNS void LANGUAGE plpgsql SET search_path=pg_catalog,public AS $$
DECLARE qualified text; guard record;
BEGIN
 qualified:=public.sytue_private_storage_table(state_table);
 FOR guard IN SELECT g.tgname FROM pg_trigger g
   WHERE g.tgrelid=state_table AND NOT g.tgisinternal
     AND g.tgfoid='public.sytue_private_storage_state_scope_guard_v2()'::regprocedure
 LOOP
  EXECUTE format('DROP TRIGGER %I ON %s',guard.tgname,qualified);
  EXECUTE format('CREATE TRIGGER %I BEFORE UPDATE OR DELETE ON %s FOR EACH ROW EXECUTE FUNCTION public.sytue_private_storage_state_scope_guard()',guard.tgname,qualified);
  EXECUTE format('ALTER TABLE %s ENABLE ALWAYS TRIGGER %I',qualified,guard.tgname);
 END LOOP;
 EXECUTE format('ALTER TABLE %s DROP COLUMN IF EXISTS releases',qualified);
END;
$$;

-- El guardián v2 se llama distinto del v1. La admisión de ámbitos (scope_admission_v1)
-- reconoce las tablas de estado por el guardián v1; sin esto, tras instalar v2 una
-- fusión de organizaciones dejaría de verlas como estado retenido. Solo se redefine
-- donde la admisión ya existe: crearla aquí la daría por instalada sin su control.
DO $do$
BEGIN
 IF to_regprocedure('sytue_scope_admission_tables()') IS NOT NULL THEN
  EXECUTE $sql$CREATE OR REPLACE FUNCTION sytue_scope_admission_tables() RETURNS TABLE(scope_table oid,state_only boolean)
LANGUAGE sql STABLE AS $fn$
 SELECT c.oid,
 EXISTS(SELECT 1 FROM pg_trigger t JOIN pg_proc f ON f.oid=t.tgfoid WHERE t.tgrelid=c.oid AND NOT t.tgisinternal
 AND (f.proname IN('sytue_private_storage_state_scope_guard','sytue_private_storage_state_scope_guard_v2') OR (f.proname='sytue_scope_admission_data_guard' AND t.tgargs=convert_to('state','UTF8')||decode('00','hex'))))
 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
 WHERE n.nspname=current_schema() AND c.relkind IN('r','p') AND NOT c.relispartition
 AND (EXISTS(SELECT 1 FROM pg_constraint k WHERE k.conrelid=c.oid AND k.conname='scope_bound_merge_contract')
 OR EXISTS(SELECT 1 FROM pg_trigger t JOIN pg_proc f ON f.oid=t.tgfoid WHERE t.tgrelid=c.oid AND NOT t.tgisinternal
 AND f.proname IN('sytue_private_storage_state_scope_guard','sytue_private_storage_state_scope_guard_v2','sytue_scope_admission_data_guard')))
 ORDER BY c.relname
$fn$;$sql$;
 END IF;
 IF to_regprocedure('sytue_scope_admission_register(regclass,boolean)') IS NOT NULL THEN
  EXECUTE $sql$CREATE OR REPLACE FUNCTION sytue_scope_admission_register(scope_table regclass,state_only boolean DEFAULT false)
RETURNS void LANGUAGE plpgsql AS $fn$
DECLARE qualified text;
BEGIN
 SELECT format('%I.%I',n.nspname,c.relname) INTO qualified
 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
 WHERE c.oid=scope_table AND n.nspname=current_schema() AND c.relkind='r'
 AND EXISTS(SELECT 1 FROM pg_attribute a WHERE a.attrelid=c.oid AND a.attname='tenant' AND a.attnum>0 AND NOT a.attisdropped);
 IF qualified IS NULL THEN RAISE EXCEPTION 'scope admission table invalid' USING ERRCODE='22023'; END IF;
 IF state_only IS NULL OR (state_only AND NOT EXISTS(SELECT 1 FROM pg_trigger t WHERE t.tgrelid=scope_table AND NOT t.tgisinternal AND t.tgenabled IN('O','A') AND t.tgtype=27 AND t.tgfoid IN (to_regprocedure('sytue_private_storage_state_scope_guard()'),to_regprocedure('sytue_private_storage_state_scope_guard_v2()'))))
 OR (NOT state_only AND NOT EXISTS(SELECT 1 FROM pg_constraint k WHERE k.conrelid=scope_table AND k.conname='scope_bound_merge_contract' AND pg_get_constraintdef(k.oid)='CHECK (sytue_scope_merge_allowed())')) THEN
  RAISE EXCEPTION 'scope admission table role invalid' USING ERRCODE='22023';
 END IF;
 EXECUTE format('CREATE TRIGGER scope_bound_admission BEFORE INSERT OR UPDATE ON %s FOR EACH ROW EXECUTE FUNCTION sytue_scope_admission_data_guard(%L)',qualified,CASE WHEN state_only THEN 'state' ELSE 'bound' END);
 EXECUTE format('CREATE CONSTRAINT TRIGGER scope_bound_admission_commit AFTER INSERT OR UPDATE ON %s DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION sytue_scope_admission_data_guard(%L)',qualified,CASE WHEN state_only THEN 'state' ELSE 'bound' END);
END $fn$;$sql$;
 END IF;
END
$do$;
