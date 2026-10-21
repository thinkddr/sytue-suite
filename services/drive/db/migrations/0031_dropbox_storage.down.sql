-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
-- Se niega a retirar el soporte mientras una organización siga usando Dropbox.
ALTER TABLE external_storage DROP CONSTRAINT IF EXISTS external_storage_kind_check;
ALTER TABLE external_storage ADD CONSTRAINT external_storage_kind_check
    CHECK (kind IN ('s3', 'webdav', 'gdrive', 'onedrive', 'nas'));

ALTER TABLE external_storage DROP CONSTRAINT IF EXISTS external_storage_oauth_instance_not_blank;
ALTER TABLE external_storage DROP CONSTRAINT IF EXISTS external_storage_endpoint_not_blank;
ALTER TABLE external_storage ADD CONSTRAINT external_storage_endpoint_not_blank CHECK (length(endpoint) > 0);
ALTER TABLE external_storage DROP CONSTRAINT IF EXISTS external_storage_bucket_not_blank;
ALTER TABLE external_storage ADD CONSTRAINT external_storage_bucket_not_blank CHECK (length(bucket) > 0);
