-- SPDX-License-Identifier: Apache-2.0
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM matrix_call_epochs) THEN
  RAISE EXCEPTION 'Encrypted call room references must be revoked before rollback';
 END IF;
END $$;
DROP TABLE matrix_call_epochs;
DROP TABLE matrix_call_members;
