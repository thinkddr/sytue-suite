-- SPDX-License-Identifier: Apache-2.0
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM matrix_storage_cutovers WHERE phase<>'legacy') THEN
  RAISE EXCEPTION 'active source cutover requires publication admission';
 END IF;
END $$;
DROP TRIGGER matrix_storage_publication_cutover_guard ON matrix_storage_cutovers;
DROP FUNCTION matrix_storage_publication_cutover_guard();
