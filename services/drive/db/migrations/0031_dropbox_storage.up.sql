-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
-- Amplía el catálogo sin modificar una migración que producción ya aplicó.
ALTER TABLE external_storage DROP CONSTRAINT IF EXISTS external_storage_kind_check;
ALTER TABLE external_storage ADD CONSTRAINT external_storage_kind_check
    CHECK (kind IN ('s3', 'webdav', 'gdrive', 'onedrive', 'dropbox', 'nas'));

-- Las restricciones originales eran de S3 y también bloqueaban WebDAV y OAuth:
-- estas clases no tienen bucket; en OAuth la conexión es la instancia consentida.
ALTER TABLE external_storage DROP CONSTRAINT IF EXISTS external_storage_endpoint_not_blank;
ALTER TABLE external_storage ADD CONSTRAINT external_storage_endpoint_not_blank
    CHECK (kind IN ('gdrive', 'onedrive', 'dropbox') OR length(btrim(endpoint)) > 0);
ALTER TABLE external_storage DROP CONSTRAINT IF EXISTS external_storage_bucket_not_blank;
ALTER TABLE external_storage ADD CONSTRAINT external_storage_bucket_not_blank
    CHECK (kind <> 's3' OR length(btrim(bucket)) > 0);
ALTER TABLE external_storage ADD CONSTRAINT external_storage_oauth_instance_not_blank
    CHECK (kind NOT IN ('gdrive', 'onedrive', 'dropbox') OR length(btrim(instance_id)) > 0);
