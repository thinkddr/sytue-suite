-- SPDX-License-Identifier: Apache-2.0
-- Immutable v1 shared implementation. Domain wrappers supply only routing.
CREATE OR REPLACE FUNCTION sytue_private_storage_table(state_table regclass)
RETURNS text LANGUAGE plpgsql STABLE SET search_path=pg_catalog,public AS $$
DECLARE qualified text;
BEGIN
 SELECT format('%I.%I',n.nspname,c.relname) INTO qualified
 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
 WHERE c.oid=state_table AND n.nspname='public' AND c.relkind='r'
   AND c.relname ~ '^[a-z][a-z0-9_]*_private_storage_states$';
 IF qualified IS NULL THEN
  RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='private_storage_state_table_invalid';
 END IF;
 RETURN qualified;
END;
$$;
CREATE OR REPLACE FUNCTION sytue_private_storage_guard(state_table regclass,t text,o text,account uuid DEFAULT NULL)
RETURNS void LANGUAGE plpgsql SET search_path=pg_catalog,public SET app.maintenance='on' AS $$
DECLARE state_mode text; qualified text;
BEGIN
 qualified:=public.sytue_private_storage_table(state_table);
 IF t IS NULL OR t !~ '^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$'
    OR o IS NULL OR o='' OR octet_length(o)>1024 THEN
  RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='private_storage_scope_invalid';
 END IF;
 EXECUTE format('SELECT mode FROM %s WHERE tenant=$1 AND owner_sub=$2 FOR SHARE',qualified)
   INTO state_mode USING t,o;
 IF state_mode IS NULL THEN
  -- Merge has already moved state rows before rewriting domain resources. Do
  -- not recreate an old-tenant ghost; existing frozen rows still reject above.
  IF current_setting('app.merge',true)='on' THEN RETURN; END IF;
  -- Row locks, not advisory-only locks: an old RR snapshot that cannot see a
  -- newly inserted state must abort at the unique upsert instead of escaping.
  IF account IS NULL THEN
   EXECUTE format('INSERT INTO %s(tenant,owner_sub) VALUES($1,$2)
    ON CONFLICT(tenant,owner_sub) DO UPDATE SET owner_sub=EXCLUDED.owner_sub RETURNING mode',qualified)
    INTO state_mode USING t,o;
  ELSE
   EXECUTE format('INSERT INTO %s(tenant,owner_sub,account_id) VALUES($1,$2,$3)
    ON CONFLICT(tenant,owner_sub) DO UPDATE SET owner_sub=EXCLUDED.owner_sub RETURNING mode',qualified)
    INTO state_mode USING t,o,account;
  END IF;
 END IF;
 IF state_mode<>'legacy' OR state_mode IS NULL THEN
  RAISE EXCEPTION USING ERRCODE='PST01',MESSAGE='private_storage_legacy_write_blocked';
 END IF;
END;
$$;
CREATE OR REPLACE FUNCTION sytue_private_storage_build_completed()
RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,public AS $$
DECLARE incomplete boolean; qualified text;
BEGIN
 qualified:=public.sytue_private_storage_table(TG_RELID::regclass);
 EXECUTE format('SELECT EXISTS(SELECT 1 FROM %s WHERE tenant=$1 AND owner_sub=$2 AND mode=''building'')',qualified)
   INTO incomplete USING NEW.tenant,NEW.owner_sub;
 IF incomplete THEN
  RAISE EXCEPTION USING ERRCODE='PST02',MESSAGE='private_storage_snapshot_incomplete';
 END IF;
 RETURN NULL;
END;
$$;
-- Register this on each domain's state table. Routing/snapshot counts that
-- already authenticated a retained source cannot be renamed or reopened by a
-- generic maintenance/merge update. Verified retirement may advance frozen to
-- private while retaining exactly the same historical identity and inventory.
CREATE OR REPLACE FUNCTION sytue_private_storage_state_scope_guard()
RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,public AS $$
BEGIN
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
 IF TG_OP='DELETE' THEN RETURN OLD; END IF;
 RETURN NEW;
END;
$$;

-- A routing-only predicate for batch maintenance. This does not replace the
-- shared row lock in the writer guard; it prevents a known frozen owner from
-- aborting work for every other legacy owner in the same maintenance batch.
CREATE OR REPLACE FUNCTION sytue_private_storage_frozen(state_table regclass,t text,o text)
RETURNS boolean LANGUAGE plpgsql STABLE SET search_path=pg_catalog,public SET app.maintenance='on' AS $$
DECLARE frozen boolean; qualified text;
BEGIN
 qualified:=public.sytue_private_storage_table(state_table);
 IF o IS NULL OR o='' THEN RETURN false; END IF;
 IF t IS NULL OR t !~ '^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$' OR octet_length(o)>1024 THEN
  RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='private_storage_scope_invalid';
 END IF;
 EXECUTE format('SELECT EXISTS(SELECT 1 FROM %s WHERE tenant=$1 AND owner_sub=$2 AND mode<>''legacy'')',qualified)
  INTO frozen USING t,o;
 RETURN frozen;
END;
$$;
CREATE OR REPLACE FUNCTION sytue_private_storage_snapshot_immutable()
RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,public AS $$
BEGIN
 RAISE EXCEPTION USING ERRCODE='PST01',MESSAGE='private_storage_source_immutable';
END;
$$;
