-- SPDX-License-Identifier: Apache-2.0
-- Office's retained historical sources; new private content stays on devices.
-- sytue:private-storage-fence-v1
DO $office$
DECLARE product text; states text; snapshot text;
BEGIN
 FOREACH product IN ARRAY ARRAY['docs','sheets','slides'] LOOP
  states:=product||'_private_storage_states';snapshot:=product||'_private_storage_snapshot';
  EXECUTE format($sql$CREATE TABLE %I(
   tenant text NOT NULL,owner_sub text NOT NULL CHECK(owner_sub<>''),
   mode text NOT NULL DEFAULT 'legacy' CHECK(mode IN('legacy','building','frozen','private')),
   epoch uuid,started_at timestamptz,source_rows bigint NOT NULL DEFAULT 0 CHECK(source_rows>=0),
   source_blobs bigint NOT NULL DEFAULT 0 CHECK(source_blobs>=0 AND source_blobs<=source_rows),
   PRIMARY KEY(tenant,owner_sub),
   CHECK((mode='legacy' AND epoch IS NULL AND started_at IS NULL) OR(mode<>'legacy' AND epoch IS NOT NULL AND started_at IS NOT NULL)))$sql$,states);
  EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',states);
  EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',states);
  EXECUTE format($sql$CREATE POLICY office_private_owner ON %I USING(tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.owner_sub',true)) WITH CHECK(tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.owner_sub',true))$sql$,states);
  EXECUTE format($sql$CREATE POLICY office_private_maintenance ON %I USING(current_setting('app.maintenance',true)='on') WITH CHECK(current_setting('app.maintenance',true)='on')$sql$,states);
  EXECUTE format($sql$CREATE POLICY office_private_merge ON %I USING(current_setting('app.merge',true)='on') WITH CHECK(current_setting('app.merge',true)='on')$sql$,states);
  EXECUTE format('CREATE TRIGGER office_private_scope_guard BEFORE UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION sytue_private_storage_state_scope_guard()',states);
  EXECUTE format('CREATE CONSTRAINT TRIGGER office_private_completed AFTER INSERT OR UPDATE ON %I DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION sytue_private_storage_build_completed()',states);
  EXECUTE format($sql$CREATE TABLE %I(
   tenant text NOT NULL,owner_sub text NOT NULL,epoch uuid NOT NULL,
   kind text COLLATE "C" NOT NULL CHECK(kind IN('document','operation','snapshot','version','permission','share_link','comment','blob','edit_request','protection','protection_key','filter_view','template','template_log','script','script_version','script_run','brand','link_policy','connection','query','office_bytes')),
   id text COLLATE "C" NOT NULL,payload bytea,size_bytes bigint NOT NULL CHECK(size_bytes>=0 AND size_bytes<=140737488355328),
   sha256 text NOT NULL CHECK(sha256 ~ '^[a-f0-9]{64}$'),storage_key text,
   PRIMARY KEY(tenant,owner_sub,epoch,kind,id),
   FOREIGN KEY(tenant,owner_sub) REFERENCES %I(tenant,owner_sub),
   CHECK((kind='office_bytes' AND payload IS NULL AND storage_key IS NOT NULL AND storage_key<>'') OR(kind<>'office_bytes' AND payload IS NOT NULL AND storage_key IS NULL AND octet_length(payload) BETWEEN 2 AND 33554432 AND size_bytes=octet_length(payload) AND sha256=encode(digest(payload,'sha256'),'hex'))))$sql$,snapshot,states);
  EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',snapshot);
  EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',snapshot);
  EXECUTE format($sql$CREATE POLICY office_private_read ON %I FOR SELECT USING(tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.owner_sub',true))$sql$,snapshot);
  EXECUTE format($sql$CREATE POLICY office_private_capture ON %I FOR INSERT WITH CHECK(tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.owner_sub',true) AND EXISTS(SELECT 1 FROM %I s WHERE s.tenant=%I.tenant AND s.owner_sub=%I.owner_sub AND s.epoch=%I.epoch AND s.mode='building'))$sql$,snapshot,states,snapshot,snapshot,snapshot);
  EXECUTE format('CREATE TRIGGER office_private_immutable BEFORE UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION sytue_private_storage_snapshot_immutable()',snapshot);
  EXECUTE format('GRANT SELECT,INSERT,UPDATE,DELETE ON %I TO docs_app',states);
  EXECUTE format('GRANT SELECT,INSERT ON %I TO docs_app',snapshot);
 END LOOP;
END $office$;

CREATE FUNCTION docs_private_product(k text) RETURNS text LANGUAGE sql IMMUTABLE AS $$
 SELECT CASE k WHEN 'document' THEN 'docs' WHEN 'spreadsheet' THEN 'sheets' WHEN 'presentation' THEN 'slides' END
$$;
CREATE FUNCTION docs_private_owner_guard(t text,o text,k text) RETURNS void LANGUAGE plpgsql
 SET search_path=pg_catalog,public SET app.maintenance='on' AS $$
DECLARE product text; state_table regclass;
BEGIN
 product:=public.docs_private_product(k);
 IF product IS NULL OR coalesce(o,'')='' THEN RETURN;END IF;
 state_table:=to_regclass('public.'||product||'_private_storage_states');
 PERFORM public.sytue_private_storage_guard(state_table,t,o);
END $$;
CREATE FUNCTION docs_private_owner_frozen(t text,o text,k text) RETURNS boolean LANGUAGE plpgsql STABLE
 SET search_path=pg_catalog,public SET app.maintenance='on' AS $$
DECLARE product text;
BEGIN
 product:=public.docs_private_product(k);IF product IS NULL THEN RETURN false;END IF;
 RETURN public.sytue_private_storage_frozen(to_regclass('public.'||product||'_private_storage_states'),t,o);
END $$;
-- External effects may perform domain SQL in another transaction. A shared
-- document routing lock permits those writes but excludes an owner/kind/id
-- reassignment (and UUID reuse) until the entire effect has finished.
CREATE FUNCTION docs_private_document_lock(doc uuid,mutate boolean) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
 IF doc IS NULL THEN RETURN;END IF;
 IF mutate THEN PERFORM pg_advisory_xact_lock(hashtextextended('docs-private-routing:'||doc::text,0));
 ELSE PERFORM pg_advisory_xact_lock_shared(hashtextextended('docs-private-routing:'||doc::text,0));END IF;
END $$;
CREATE FUNCTION docs_private_row_guard() RETURNS trigger LANGUAGE plpgsql
 SET search_path=pg_catalog,public SET app.maintenance='on' AS $$
DECLARE value jsonb; t text; o text; k text; doc uuid;
BEGIN
 FOR value IN SELECT v FROM(VALUES(CASE WHEN TG_OP<>'INSERT' THEN to_jsonb(OLD) END),(CASE WHEN TG_OP<>'DELETE' THEN to_jsonb(NEW) END))x(v) WHERE v IS NOT NULL LOOP
  t:=value->>'tenant';o:=NULL;k:=NULL;
  IF TG_TABLE_NAME='documents' THEN
   PERFORM public.docs_private_document_lock((value->>'id')::uuid,TG_OP='INSERT' OR (TG_OP='UPDATE' AND (OLD.id IS DISTINCT FROM NEW.id OR OLD.tenant IS DISTINCT FROM NEW.tenant OR OLD.owner_sub IS DISTINCT FROM NEW.owner_sub OR OLD.kind IS DISTINCT FROM NEW.kind)));
  ELSIF value ? 'document_id' THEN PERFORM public.docs_private_document_lock((value->>'document_id')::uuid,false);END IF;
  IF TG_TABLE_NAME='documents' THEN o:=value->>'owner_sub';k:=value->>'kind';
  ELSIF TG_TABLE_NAME='doc_scripts' THEN
   o:=value->>'author_sub';k:=value->>'kind';
  ELSIF TG_TABLE_NAME='doc_script_versions' THEN
   SELECT author_sub,kind INTO o,k FROM public.doc_scripts WHERE tenant=t AND id=(value->>'script_id')::uuid;
  ELSIF TG_TABLE_NAME IN('data_connections','data_queries') THEN
   o:=value->>'created_by';k:='spreadsheet';
  ELSE
   doc:=(value->>'document_id')::uuid;
   SELECT owner_sub,kind::text INTO o,k FROM public.documents WHERE tenant=t AND id=doc;
  END IF;
  IF TG_TABLE_NAME='data_queries' THEN
   PERFORM public.docs_private_owner_guard(t,(SELECT created_by FROM public.data_connections WHERE tenant=t AND id=(value->>'connection_id')::uuid),'spreadsheet');
  END IF;
  PERFORM public.docs_private_owner_guard(t,o,k);
  -- Personal filter views remain personal even on a document owned by somebody
  -- else; the document owner's source never includes another person's views.
  IF TG_TABLE_NAME='doc_vistas_filtro' THEN PERFORM public.docs_private_owner_guard(t,value->>'sub',k);END IF;
  IF TG_TABLE_NAME='doc_script_runs' THEN
   SELECT author_sub,kind INTO o,k FROM public.doc_scripts WHERE tenant=t AND id=(value->>'script_id')::uuid;
   PERFORM public.docs_private_owner_guard(t,o,k);
   PERFORM public.docs_private_owner_guard(t,value->>'sub',k);
  END IF;
 END LOOP;
 IF TG_OP='DELETE' THEN RETURN OLD;END IF;RETURN NEW;
END $$;

DO $office$
DECLARE relation text;
BEGIN
 FOREACH relation IN ARRAY ARRAY['documents','doc_operations','doc_snapshots','doc_versions','doc_permissions','doc_share_links','doc_comments','doc_blobs','doc_peticiones_edicion','doc_proteccion','doc_proteccion_claves','doc_vistas_filtro','doc_org_templates','doc_org_template_log','doc_scripts','doc_script_versions','doc_script_runs','data_connections','data_queries'] LOOP
  EXECUTE format('CREATE TRIGGER office_private_write BEFORE INSERT OR UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION docs_private_row_guard()',relation);
 END LOOP;
END $office$;

CREATE FUNCTION docs_private_source_insert_guard() RETURNS trigger LANGUAGE plpgsql
 SET search_path=pg_catalog,public SET app.maintenance='on' AS $$
DECLARE state_table text; allowed boolean;
BEGIN
 IF TG_TABLE_NAME NOT IN('docs_private_storage_snapshot','sheets_private_storage_snapshot','slides_private_storage_snapshot') THEN RAISE EXCEPTION 'invalid Office source table' USING ERRCODE='PST01';END IF;
 state_table:=replace(TG_TABLE_NAME,'_snapshot','_states');
 EXECUTE format('SELECT EXISTS(SELECT 1 FROM public.%I WHERE tenant=$1 AND owner_sub=$2 AND epoch=$3 AND mode=''building'')',state_table) INTO allowed USING NEW.tenant,NEW.owner_sub,NEW.epoch;
 IF NOT allowed THEN RAISE EXCEPTION 'private_storage_source_immutable' USING ERRCODE='PST01';END IF;
 RETURN NEW;
END $$;
CREATE FUNCTION docs_private_retained_truncate() RETURNS trigger LANGUAGE plpgsql
 SET search_path=pg_catalog,public SET app.maintenance='on' AS $$
BEGIN
 IF EXISTS(SELECT 1 FROM public.docs_private_storage_states WHERE mode<>'legacy') OR EXISTS(SELECT 1 FROM public.sheets_private_storage_states WHERE mode<>'legacy') OR EXISTS(SELECT 1 FROM public.slides_private_storage_states WHERE mode<>'legacy') THEN
  RAISE EXCEPTION 'Office retained source cannot be truncated' USING ERRCODE='PST01';
 END IF;RETURN NULL;
END $$;
DO $office$
DECLARE product text; relation text;
BEGIN
 FOREACH product IN ARRAY ARRAY['docs','sheets','slides'] LOOP
  EXECUTE format('CREATE TRIGGER office_private_insert BEFORE INSERT ON %I FOR EACH ROW EXECUTE FUNCTION docs_private_source_insert_guard()',product||'_private_storage_snapshot');
  FOREACH relation IN ARRAY ARRAY[product||'_private_storage_states',product||'_private_storage_snapshot'] LOOP
   EXECUTE format('CREATE TRIGGER office_private_no_truncate BEFORE TRUNCATE ON %I FOR EACH STATEMENT EXECUTE FUNCTION docs_private_retained_truncate()',relation);
  END LOOP;
 END LOOP;
END $office$;
-- sytue:scope-bound-merge-v1
SELECT sytue_scope_merge_register('docs_private_storage_snapshot'::regclass);
SELECT sytue_scope_merge_register('sheets_private_storage_snapshot'::regclass);
SELECT sytue_scope_merge_register('slides_private_storage_snapshot'::regclass);
-- sytue:scope-admission-v1
GRANT SELECT,INSERT ON private_scope_admission_control TO docs_app;
SELECT sytue_scope_admission_register_existing();

DO $office$
DECLARE relation text;
BEGIN
 FOREACH relation IN ARRAY ARRAY['documents','doc_operations','doc_snapshots','doc_versions','doc_permissions','doc_share_links','doc_comments','doc_blobs','doc_peticiones_edicion','doc_proteccion','doc_proteccion_claves','doc_vistas_filtro','doc_org_templates','doc_org_template_log','doc_scripts','doc_script_versions','doc_script_runs','data_connections','data_queries'] LOOP
  EXECUTE format('CREATE TRIGGER office_private_legacy_no_truncate BEFORE TRUNCATE ON %I FOR EACH STATEMENT EXECUTE FUNCTION docs_private_retained_truncate()',relation);
 END LOOP;
END $office$;
