-- SPDX-License-Identifier: Apache-2.0
BEGIN;
SELECT set_config('app.maintenance','on',true);
LOCK TABLE matrix_storage_cutovers IN ACCESS EXCLUSIVE MODE;
-- Never lose a durable fence while an older server could reopen source rooms.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM matrix_storage_cutovers) THEN
  RAISE EXCEPTION 'matrix_storage_cutover_must_be_retained';
 END IF;
END; $$;
DROP TABLE matrix_storage_cutovers;
DROP FUNCTION matrix_storage_cutover_monotonic();
COMMIT;
