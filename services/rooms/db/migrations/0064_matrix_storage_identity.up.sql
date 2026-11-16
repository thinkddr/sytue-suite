-- SPDX-License-Identifier: Apache-2.0
-- A separate routing table also protects a rolling deployment: an older Rooms
-- binary must not select these rooms and invite its legacy human identity.
CREATE TABLE matrix_storage_isolated_rooms
    (LIKE matrix_storage_rooms INCLUDING ALL);
ALTER TABLE matrix_storage_isolated_rooms ENABLE ROW LEVEL SECURITY;
ALTER TABLE matrix_storage_isolated_rooms FORCE ROW LEVEL SECURITY;
CREATE POLICY matrix_storage_isolated_rooms_tenant ON matrix_storage_isolated_rooms
    USING (tenant=current_setting('app.tenant',true))
    WITH CHECK (tenant=current_setting('app.tenant',true));
CREATE POLICY matrix_storage_isolated_rooms_maintenance ON matrix_storage_isolated_rooms
    USING (current_setting('app.maintenance',true)='on')
    WITH CHECK (current_setting('app.maintenance',true)='on');
CREATE POLICY matrix_storage_isolated_rooms_platform_merge ON matrix_storage_isolated_rooms
    USING (current_setting('app.merge',true)='on')
    WITH CHECK (current_setting('app.merge',true)='on');
ALTER TABLE matrix_native_consents ADD COLUMN identity_version smallint NOT NULL DEFAULT 1
    CHECK (identity_version IN (1,2));
