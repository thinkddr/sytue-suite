-- SPDX-License-Identifier: Apache-2.0
DO $$ BEGIN IF EXISTS(SELECT 1 FROM matrix_document_identity_access WHERE version>0) THEN RAISE EXCEPTION 'An identity fence cannot be discarded';END IF;END $$;
ALTER TABLE matrix_call_members DROP COLUMN identity_version;
DROP TABLE matrix_document_identity_access;
