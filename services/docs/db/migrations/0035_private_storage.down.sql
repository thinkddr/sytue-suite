-- SPDX-License-Identifier: Apache-2.0
DO $office$
DECLARE relation text;product text;
BEGIN
 IF EXISTS(SELECT 1 FROM docs_private_storage_states WHERE mode<>'legacy') OR EXISTS(SELECT 1 FROM sheets_private_storage_states WHERE mode<>'legacy') OR EXISTS(SELECT 1 FROM slides_private_storage_states WHERE mode<>'legacy') THEN RAISE EXCEPTION 'Office retained source is immutable' USING ERRCODE='PST01';END IF;
 FOREACH relation IN ARRAY ARRAY['documents','doc_operations','doc_snapshots','doc_versions','doc_permissions','doc_share_links','doc_comments','doc_blobs','doc_peticiones_edicion','doc_proteccion','doc_proteccion_claves','doc_vistas_filtro','doc_org_templates','doc_org_template_log','doc_scripts','doc_script_versions','doc_script_runs','data_connections','data_queries'] LOOP
  EXECUTE format('DROP TRIGGER IF EXISTS office_private_write ON %I',relation);
  EXECUTE format('DROP TRIGGER IF EXISTS office_private_legacy_no_truncate ON %I',relation);
 END LOOP;
 FOREACH product IN ARRAY ARRAY['docs','sheets','slides'] LOOP
  EXECUTE format('DROP TABLE IF EXISTS %I',product||'_private_storage_snapshot');
  EXECUTE format('DROP TABLE IF EXISTS %I',product||'_private_storage_states');
 END LOOP;
END $office$;
DROP FUNCTION IF EXISTS docs_private_source_insert_guard();
DROP FUNCTION IF EXISTS docs_private_retained_truncate();
DROP FUNCTION IF EXISTS docs_private_row_guard();
DROP FUNCTION IF EXISTS docs_private_owner_frozen(text,text,text);
DROP FUNCTION IF EXISTS docs_private_owner_guard(text,text,text);
DROP FUNCTION IF EXISTS docs_private_document_lock(uuid,boolean);
DROP FUNCTION IF EXISTS docs_private_product(text);
