-- SPDX-License-Identifier: Apache-2.0
-- Do not downgrade while versioned external keys exist: the older router cannot
-- resolve them. Operational rollback must retain the new routing migration.
BEGIN;
SELECT set_config('app.maintenance','on',true);
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM files WHERE storage_key LIKE 'ext/v1/%')
       OR EXISTS (SELECT 1 FROM file_versions WHERE storage_key LIKE 'ext/v1/%')
       OR EXISTS (SELECT 1 FROM cipher_blobs WHERE storage_key LIKE 'ext/v1/%') THEN
        RAISE EXCEPTION 'versioned external objects still require location routing';
    END IF;
END $$;
DROP TRIGGER external_storage_pin_location ON external_storage;
DROP FUNCTION pin_external_storage_location();
DROP TRIGGER external_storage_archive_location ON external_storage;
DROP FUNCTION archive_external_storage_location();
DROP TABLE external_storage_locations;
ALTER TABLE external_storage DROP COLUMN location_id;

COMMIT;
