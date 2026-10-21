-- SPDX-License-Identifier: Apache-2.0
-- Historical identity fences keep their original organization scope, including
-- after a merge. A proof for the destination must be freshly bound to that scope.
CREATE TABLE matrix_document_identity_access(org_slug text NOT NULL,subject text NOT NULL,version bigint NOT NULL DEFAULT 0,PRIMARY KEY(org_slug,subject));
ALTER TABLE matrix_document_identity_access ENABLE ROW LEVEL SECURITY;
ALTER TABLE matrix_document_identity_access FORCE ROW LEVEL SECURITY;
CREATE POLICY matrix_document_identity_access_tenant ON matrix_document_identity_access USING(org_slug=current_setting('app.tenant',true)) WITH CHECK(org_slug=current_setting('app.tenant',true));
CREATE POLICY matrix_document_identity_access_internal ON matrix_document_identity_access USING(current_setting('app.maintenance',true)='on') WITH CHECK(current_setting('app.maintenance',true)='on');
ALTER TABLE matrix_call_members ADD COLUMN identity_version bigint NOT NULL DEFAULT 0;
