-- SPDX-License-Identifier: Apache-2.0
BEGIN;
SET LOCAL row_security=off;
LOCK TABLE matrix_storage_device_fences IN ACCESS EXCLUSIVE MODE;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM matrix_storage_device_fences) THEN
  RAISE EXCEPTION 'storage device fences must be retained; rollback refused' USING ERRCODE='55000';
 END IF;
END $$;
DROP TABLE matrix_storage_device_fences;
DROP FUNCTION matrix_storage_device_fence_guard();
COMMIT;
