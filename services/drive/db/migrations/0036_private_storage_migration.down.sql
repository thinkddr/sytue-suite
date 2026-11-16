-- SPDX-License-Identifier: Apache-2.0
-- Never silently reopen legacy writers or discard an unfinished encrypted migration.
DO $$ BEGIN
    PERFORM set_config('app.maintenance','on',true);
    IF EXISTS(SELECT 1 FROM drive_private_storage_states WHERE mode<>'legacy') THEN
        RAISE EXCEPTION 'drive: private migration must be recovered before downgrade';
    END IF;
END $$;
DROP TRIGGER drive_private_files_guard ON files;
DROP TRIGGER drive_private_folders_guard ON folders;
DROP TRIGGER drive_private_versions_guard ON file_versions;
DROP TRIGGER drive_private_shares_guard ON shares;
DROP TRIGGER drive_private_favorites_guard ON favorites;
DROP TRIGGER drive_private_requests_guard ON drive_peticiones_edicion;
DROP TRIGGER drive_private_provenance_guard ON drive_file_provenance;
DROP FUNCTION drive_private_storage_row_guard();
DROP FUNCTION drive_private_storage_resource_guard(text,uuid);
DROP FUNCTION drive_private_storage_legacy_guard(text,text);
DROP TRIGGER drive_private_storage_build_completed ON drive_private_storage_states;
DROP FUNCTION drive_private_storage_build_completed();
DROP TABLE drive_private_storage_source_blobs;
DROP TABLE drive_private_storage_snapshot;
DROP TABLE drive_private_storage_states;
DROP INDEX drive_private_folders_source;
DROP INDEX drive_private_files_source;
DROP INDEX drive_private_requests_source;
