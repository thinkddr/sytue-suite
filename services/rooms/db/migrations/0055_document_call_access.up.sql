-- SPDX-License-Identifier: Apache-2.0
-- Document UUIDs are globally unique and the fence survives organization moves.
CREATE TABLE matrix_document_access(document_id uuid PRIMARY KEY,version bigint NOT NULL DEFAULT 0);
ALTER TABLE matrix_document_access ENABLE ROW LEVEL SECURITY;
ALTER TABLE matrix_document_access FORCE ROW LEVEL SECURITY;
CREATE POLICY matrix_document_access_internal ON matrix_document_access
 USING(current_setting('app.maintenance',true)='on') WITH CHECK(current_setting('app.maintenance',true)='on');
ALTER TABLE matrix_call_members ADD COLUMN document_access_until timestamptz;
