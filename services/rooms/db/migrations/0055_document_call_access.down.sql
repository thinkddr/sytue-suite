-- SPDX-License-Identifier: Apache-2.0
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM matrix_document_access WHERE version>0) THEN RAISE EXCEPTION 'Document access fences cannot be discarded while their documents remain usable';END IF;
END $$;
ALTER TABLE matrix_call_members DROP COLUMN document_access_until;
DROP TABLE matrix_document_access;
