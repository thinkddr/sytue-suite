-- SPDX-License-Identifier: Apache-2.0
BEGIN;
-- A newer restrictive policy may hide rows from an older rollback. Refuse if
-- the migration role cannot inspect every row, even when its SELECT is empty.
SET LOCAL row_security=off;
SELECT set_config('app.maintenance','on',true);
SELECT set_config('app.merge','on',true);
-- Never discard a new account's routing or turn its pending grant into a
-- legacy authorization. Migrate explicitly before rolling this schema back.
LOCK TABLE matrix_storage_isolated_rooms, matrix_native_consents IN ACCESS EXCLUSIVE MODE;
DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM matrix_storage_isolated_rooms)
       OR EXISTS(SELECT 1 FROM matrix_native_consents WHERE identity_version<>1) THEN
        RAISE EXCEPTION 'isolated Matrix storage still exists; rollback refused';
    END IF;
END $$;
DROP TABLE matrix_storage_isolated_rooms;
ALTER TABLE matrix_native_consents DROP COLUMN identity_version;
COMMIT;
