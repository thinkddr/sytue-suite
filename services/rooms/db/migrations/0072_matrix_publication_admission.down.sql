-- SPDX-License-Identifier: Apache-2.0
-- Retained sending/accepted evidence must be reconciled before rollback.
SELECT set_config('row_security','off',true);
DO $ BEGIN
 IF EXISTS(SELECT 1 FROM matrix_storage_publications) THEN
  RAISE EXCEPTION 'publication evidence requires explicit retirement before rollback' USING ERRCODE='55000';
 END IF;
END $$;
DROP TRIGGER matrix_storage_publication_fence_guard ON matrix_storage_device_fences;
DROP FUNCTION matrix_storage_publication_fence_guard();
DROP TABLE matrix_storage_publications;
DROP FUNCTION matrix_storage_publication_guard();
