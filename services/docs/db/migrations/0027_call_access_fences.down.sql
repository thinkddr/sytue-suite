-- SPDX-License-Identifier: Apache-2.0
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM doc_call_guard_state WHERE enabled) THEN RAISE EXCEPTION 'Disable and revoke encrypted document calls before rollback';END IF;
END $$;
DROP TRIGGER call_access_revoked ON doc_call_access_fences;
DROP TRIGGER links_call_access ON doc_share_links;
DROP TRIGGER permissions_call_access ON doc_permissions;
DROP TRIGGER documents_call_access ON documents;
DROP FUNCTION docs_assert_call_revocation();
DROP FUNCTION docs_touch_call_access();
DROP FUNCTION docs_record_call_fence(uuid,text);
DROP TABLE doc_call_access_fences;
DROP TABLE doc_call_guard_state;
DROP SEQUENCE doc_call_access_version_seq;
