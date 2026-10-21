-- SPDX-License-Identifier: Apache-2.0
-- Non-transactional versions prevent a rolled-back ACL change reopening a call.
CREATE SEQUENCE doc_call_access_version_seq;
CREATE TABLE doc_call_guard_state(singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),enabled boolean NOT NULL DEFAULT false);
INSERT INTO doc_call_guard_state(singleton) VALUES(true);
CREATE TABLE doc_call_access_fences(
 document_id uuid PRIMARY KEY,
 tenant text NOT NULL,
 version bigint NOT NULL,
 acknowledged_version bigint NOT NULL DEFAULT 0,
 transaction_id xid8 NOT NULL
);
CREATE INDEX doc_call_access_changed ON doc_call_access_fences(tenant,transaction_id);
ALTER TABLE doc_call_access_fences ENABLE ROW LEVEL SECURITY;
ALTER TABLE doc_call_access_fences FORCE ROW LEVEL SECURITY;
CREATE POLICY doc_call_access_fences_tenant ON doc_call_access_fences USING(tenant=current_setting('app.tenant',true)) WITH CHECK(tenant=current_setting('app.tenant',true));
CREATE POLICY doc_call_access_fences_platform_merge ON doc_call_access_fences USING(current_setting('app.merge',true)='on') WITH CHECK(current_setting('app.merge',true)='on');
ALTER TABLE doc_call_guard_state ENABLE ROW LEVEL SECURITY;
ALTER TABLE doc_call_guard_state FORCE ROW LEVEL SECURITY;
CREATE POLICY doc_call_guard_read ON doc_call_guard_state FOR SELECT USING(true);
CREATE POLICY doc_call_guard_configure ON doc_call_guard_state FOR UPDATE USING(current_setting('app.maintenance',true)='on') WITH CHECK(current_setting('app.maintenance',true)='on');
GRANT SELECT,INSERT,UPDATE,DELETE ON doc_call_access_fences TO docs_app;
GRANT SELECT,UPDATE ON doc_call_guard_state TO docs_app;
GRANT USAGE,SELECT ON SEQUENCE doc_call_access_version_seq TO docs_app;

CREATE FUNCTION docs_record_call_fence(doc uuid,org text) RETURNS void LANGUAGE sql AS $$
 INSERT INTO doc_call_access_fences(document_id,tenant,version,transaction_id)
 VALUES(doc,org,nextval('doc_call_access_version_seq'),pg_current_xact_id())
 ON CONFLICT(document_id) DO UPDATE SET tenant=EXCLUDED.tenant,version=EXCLUDED.version,transaction_id=EXCLUDED.transaction_id;
$$;
CREATE FUNCTION docs_touch_call_access() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE doc uuid; org text;
BEGIN
 IF TG_TABLE_NAME='documents' THEN
  IF TG_OP='UPDATE' AND OLD.id IS NOT DISTINCT FROM NEW.id AND OLD.tenant IS NOT DISTINCT FROM NEW.tenant
   AND OLD.owner_sub IS NOT DISTINCT FROM NEW.owner_sub
   AND OLD.deleted_at IS NOT DISTINCT FROM NEW.deleted_at
   AND OLD.source_ref IS NOT DISTINCT FROM NEW.source_ref THEN RETURN NULL; END IF;
  IF TG_OP='DELETE' THEN doc=OLD.id;org=OLD.tenant;ELSE doc=NEW.id;org=NEW.tenant;END IF;
 ELSE
  IF TG_OP='DELETE' THEN doc=OLD.document_id;org=OLD.tenant;ELSE doc=NEW.document_id;org=NEW.tenant;END IF;
 END IF;
 IF TG_OP='UPDATE' THEN
  IF TG_TABLE_NAME='documents' THEN
   IF OLD.id IS DISTINCT FROM NEW.id THEN PERFORM docs_record_call_fence(OLD.id,OLD.tenant);END IF;
  ELSIF OLD.document_id IS DISTINCT FROM NEW.document_id THEN PERFORM docs_record_call_fence(OLD.document_id,OLD.tenant);END IF;
 END IF;
 PERFORM docs_record_call_fence(doc,org);
 RETURN NULL;
END $$;
CREATE TRIGGER documents_call_access AFTER UPDATE OR DELETE ON documents FOR EACH ROW EXECUTE FUNCTION docs_touch_call_access();
CREATE TRIGGER permissions_call_access AFTER INSERT OR UPDATE OR DELETE ON doc_permissions FOR EACH ROW EXECUTE FUNCTION docs_touch_call_access();
CREATE TRIGGER links_call_access AFTER INSERT OR UPDATE OR DELETE ON doc_share_links FOR EACH ROW EXECUTE FUNCTION docs_touch_call_access();

-- Imports, maintenance tools and organization moves cannot silently bypass the
-- synchronous Rooms revocation, even if they do not use Store.tenantTx.
CREATE FUNCTION docs_assert_call_revocation() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE guarded boolean; pending boolean;
BEGIN
 SELECT enabled INTO guarded FROM doc_call_guard_state WHERE singleton;
 IF NOT guarded THEN RETURN NULL;END IF;
 SELECT version>acknowledged_version INTO pending FROM doc_call_access_fences WHERE document_id=NEW.document_id;
 IF pending THEN RAISE EXCEPTION 'Encrypted document calls must be revoked before committing access changes' USING ERRCODE='23514';END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER call_access_revoked
 AFTER INSERT OR UPDATE ON doc_call_access_fences DEFERRABLE INITIALLY DEFERRED
 FOR EACH ROW EXECUTE FUNCTION docs_assert_call_revocation();

REVOKE ALL ON FUNCTION docs_record_call_fence(uuid,text),docs_touch_call_access(),docs_assert_call_revocation() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION docs_record_call_fence(uuid,text),docs_touch_call_access(),docs_assert_call_revocation() TO docs_app;
