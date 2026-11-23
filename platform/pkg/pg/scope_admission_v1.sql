-- SPDX-License-Identifier: Apache-2.0
-- Infrastructure state, not application rows: indexed control records contain
-- only merge IDs, public scope names and transaction generations. No key, content,
-- ciphertext, pin or Matrix reference is stored here or relabelled by a merge.
CREATE TABLE private_scope_admission_control(
 scope_key text PRIMARY KEY CHECK(scope_key ~ '^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$'),
 state text NOT NULL DEFAULT 'open' CHECK(state IN('open','closed','retired')),
 generation bigint NOT NULL DEFAULT 0 CHECK(generation>=0),
 merge_id uuid,merge_role text CHECK(merge_role IN('source','target')),peer text,
 transition_xid xid8,
 CHECK((generation=0 AND state='open' AND merge_id IS NULL AND merge_role IS NULL AND peer IS NULL AND transition_xid IS NULL)
 OR (generation>0 AND merge_id IS NOT NULL AND merge_role IS NOT NULL AND peer IS NOT NULL AND peer<>scope_key AND transition_xid IS NOT NULL))
);
ALTER TABLE private_scope_admission_control ENABLE ROW LEVEL SECURITY;
ALTER TABLE private_scope_admission_control FORCE ROW LEVEL SECURITY;
CREATE POLICY private_scope_admission_internal ON private_scope_admission_control
 USING(current_setting('app.scope_admission_internal',true)='on')
 WITH CHECK(current_setting('app.scope_admission_internal',true)='on');

CREATE FUNCTION sytue_scope_admission_control_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' OR current_setting('app.scope_admission_internal',true) IS DISTINCT FROM 'on'
 OR OLD.scope_key IS DISTINCT FROM NEW.scope_key OR NEW.generation<=OLD.generation
 OR OLD.state='retired' THEN
  RAISE EXCEPTION 'scope admission history is monotonic and retained' USING ERRCODE='PST03';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER scope_admission_control_guard BEFORE UPDATE OR DELETE ON private_scope_admission_control
 FOR EACH ROW EXECUTE FUNCTION sytue_scope_admission_control_guard();

CREATE FUNCTION sytue_scope_admission_check(scope_tenant text,protect_commit boolean DEFAULT false) RETURNS void
LANGUAGE plpgsql SET app.scope_admission_internal='on' AS $$
DECLARE entry private_scope_admission_control%ROWTYPE;snapshot text;
BEGIN
 IF scope_tenant IS NULL OR scope_tenant !~ '^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$' THEN
  RAISE EXCEPTION 'scope admission tenant invalid' USING ERRCODE='22023';
 END IF;
 -- A previously unseen scope acquires its first row before any protected
 -- write. A concurrent close waits for that writer, then scans its committed
 -- data. Existing scopes use an indexed lookup and only lock at COMMIT.
 INSERT INTO private_scope_admission_control(scope_key) VALUES(scope_tenant) ON CONFLICT(scope_key) DO NOTHING;
 IF protect_commit THEN
  SELECT * INTO entry FROM private_scope_admission_control WHERE scope_key=scope_tenant FOR SHARE;
 ELSE
  SELECT * INTO entry FROM private_scope_admission_control WHERE scope_key=scope_tenant;
 END IF;
 IF NOT FOUND THEN RAISE EXCEPTION 'scope admission control unavailable' USING ERRCODE='40001'; END IF;
 IF entry.state<>'open' THEN
  RAISE EXCEPTION 'scope admission closed or retired' USING ERRCODE='PST03';
 END IF;
 IF entry.generation=0 THEN RETURN; END IF;
 snapshot:=nullif(current_setting('app.scope_admission_snapshot',true),'');
 IF snapshot IS NULL OR NOT coalesce(pg_visible_in_snapshot(entry.transition_xid,snapshot::pg_snapshot),false) THEN
  RAISE EXCEPTION 'scope admission generation changed; retry with fresh authority' USING ERRCODE='40001';
 END IF;
END $$;

CREATE FUNCTION sytue_scope_admission_data_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 -- A generic merge can move legacy metadata without sharing the closed
 -- admission capability. Any transition into/between bound states is guarded.
 IF TG_ARGV[0]='state' THEN
  IF NEW.mode='legacy' THEN
   IF TG_OP='INSERT' THEN RETURN NEW; END IF;
   IF OLD.mode='legacy' THEN RETURN NEW; END IF;
  END IF;
 END IF;
 PERFORM sytue_scope_admission_check(NEW.tenant,TG_WHEN='AFTER');
 RETURN NEW;
END $$;

CREATE FUNCTION sytue_scope_admission_register(scope_table regclass,state_only boolean DEFAULT false)
RETURNS void LANGUAGE plpgsql AS $$
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
END $$;

CREATE FUNCTION sytue_scope_admission_transition(job uuid,source_tenant text,target_tenant text,phase text)
RETURNS void LANGUAGE plpgsql SET app.scope_admission_internal='on' AS $$
DECLARE s private_scope_admission_control%ROWTYPE;t private_scope_admission_control%ROWTYPE;source_closed boolean;target_closed boolean;
BEGIN
 IF current_setting('app.merge',true) IS DISTINCT FROM 'on' OR job IS NULL OR job='00000000-0000-0000-0000-000000000000'::uuid
 OR source_tenant IS NULL OR target_tenant IS NULL OR source_tenant=target_tenant
 OR source_tenant !~ '^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$'
 OR target_tenant !~ '^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$'
 OR phase IS NULL OR phase NOT IN('close','abort','complete') THEN
  RAISE EXCEPTION 'scope admission transition invalid' USING ERRCODE='22023';
 END IF;
 -- Every pair takes row locks in the same order; unrelated scopes never share
 -- a lock or fetch/decompress the history of other tenants.
 INSERT INTO private_scope_admission_control(scope_key) SELECT x FROM unnest(ARRAY[source_tenant,target_tenant])x ORDER BY x ON CONFLICT(scope_key) DO NOTHING;
 PERFORM scope_key FROM private_scope_admission_control WHERE scope_key IN(source_tenant,target_tenant) ORDER BY scope_key FOR UPDATE;
 SELECT * INTO STRICT s FROM private_scope_admission_control WHERE scope_key=source_tenant;
 SELECT * INTO STRICT t FROM private_scope_admission_control WHERE scope_key=target_tenant;
 source_closed:=s.state='closed' AND s.merge_id=job AND s.merge_role='source' AND s.peer=target_tenant;
 target_closed:=t.state='closed' AND t.merge_id=job AND t.merge_role='target' AND t.peer=source_tenant;
 IF phase='close' THEN
  IF source_closed AND target_closed THEN RETURN; END IF;
  IF s.state<>'open' OR t.state<>'open' THEN
   RAISE EXCEPTION 'scope admission belongs to another or retired merge' USING ERRCODE='PST03';
  END IF;
 ELSIF phase='abort' THEN
  IF NOT source_closed AND NOT target_closed THEN RETURN; END IF;
  IF NOT source_closed OR NOT target_closed THEN RAISE EXCEPTION 'scope admission pair mismatch' USING ERRCODE='PST03'; END IF;
 ELSE
  -- An old completion must not reopen the target of a later merge.
  IF s.state='retired' AND s.merge_id=job AND s.peer=target_tenant THEN RETURN; END IF;
  IF NOT source_closed OR NOT target_closed THEN RAISE EXCEPTION 'scope admission completion mismatch' USING ERRCODE='PST03'; END IF;
 END IF;
 UPDATE private_scope_admission_control SET state=CASE phase WHEN 'close' THEN 'closed' WHEN 'abort' THEN 'open' ELSE 'retired' END,
 generation=generation+1,merge_id=job,merge_role='source',peer=target_tenant,transition_xid=pg_current_xact_id() WHERE scope_key=source_tenant;
 UPDATE private_scope_admission_control SET state=CASE phase WHEN 'close' THEN 'closed' ELSE 'open' END,
 generation=generation+1,merge_id=job,merge_role='target',peer=source_tenant,transition_xid=pg_current_xact_id() WHERE scope_key=target_tenant;
END $$;

-- Role discovery survives a missing/disabled trigger, so downgrade and merge
-- validation never turn a retained scope into ordinary movable domain rows.
CREATE FUNCTION sytue_scope_admission_tables() RETURNS TABLE(scope_table oid,state_only boolean)
LANGUAGE sql STABLE AS $$
 SELECT c.oid,
 EXISTS(SELECT 1 FROM pg_trigger t JOIN pg_proc f ON f.oid=t.tgfoid WHERE t.tgrelid=c.oid AND NOT t.tgisinternal
 AND (f.proname IN('sytue_private_storage_state_scope_guard','sytue_private_storage_state_scope_guard_v2') OR (f.proname='sytue_scope_admission_data_guard' AND t.tgargs=convert_to('state','UTF8')||decode('00','hex'))))
 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
 WHERE n.nspname=current_schema() AND c.relkind IN('r','p') AND NOT c.relispartition
 AND (EXISTS(SELECT 1 FROM pg_constraint k WHERE k.conrelid=c.oid AND k.conname='scope_bound_merge_contract')
 OR EXISTS(SELECT 1 FROM pg_trigger t JOIN pg_proc f ON f.oid=t.tgfoid WHERE t.tgrelid=c.oid AND NOT t.tgisinternal
 AND f.proname IN('sytue_private_storage_state_scope_guard','sytue_private_storage_state_scope_guard_v2','sytue_scope_admission_data_guard')))
 ORDER BY c.relname
$$;

CREATE FUNCTION sytue_scope_admission_register_existing() RETURNS void LANGUAGE plpgsql AS $$
DECLARE tab record;
BEGIN
 FOR tab IN SELECT * FROM sytue_scope_admission_tables() LOOP
  PERFORM sytue_scope_admission_register(tab.scope_table::regclass,tab.state_only);
 END LOOP;
END $$;

CREATE FUNCTION sytue_scope_admission_remove() RETURNS void LANGUAGE plpgsql
 SET row_security=off SET app.scope_admission_internal='on' AS $$
DECLARE tab record;present boolean;
BEGIN
 -- Table locks precede control locks, matching a protected writer's statement
 -- then deferred-commit order. All existence checks run after these locks.
 FOR tab IN SELECT * FROM sytue_scope_admission_tables() LOOP
  EXECUTE format('LOCK TABLE %s IN ACCESS EXCLUSIVE MODE',tab.scope_table::regclass);
 END LOOP;
 LOCK TABLE private_scope_admission_control IN ACCESS EXCLUSIVE MODE;
 IF EXISTS(SELECT 1 FROM private_scope_admission_control WHERE generation>0) THEN RAISE EXCEPTION 'scope admission history is retained' USING ERRCODE='PST03'; END IF;
 FOR tab IN SELECT * FROM sytue_scope_admission_tables() LOOP
  IF tab.state_only THEN
   EXECUTE format('SELECT EXISTS(SELECT 1 FROM %s WHERE mode<>''legacy'')',tab.scope_table::regclass) INTO present;
  ELSE
   EXECUTE format('SELECT EXISTS(SELECT 1 FROM %s)',tab.scope_table::regclass) INTO present;
  END IF;
  IF present THEN RAISE EXCEPTION 'scope admission data is retained' USING ERRCODE='PST03'; END IF;
 END LOOP;
 FOR tab IN SELECT * FROM sytue_scope_admission_tables() LOOP
  EXECUTE format('DROP TRIGGER IF EXISTS scope_bound_admission ON %s',tab.scope_table::regclass);
  EXECUTE format('DROP TRIGGER IF EXISTS scope_bound_admission_commit ON %s',tab.scope_table::regclass);
 END LOOP;
END $$;
