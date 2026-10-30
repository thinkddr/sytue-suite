-- SPDX-License-Identifier: Apache-2.0
-- Private application rooms contain ciphertext in Matrix. This table contains
-- routing references only; it never contains object content or encryption keys.
CREATE TABLE matrix_storage_rooms (
    tenant text NOT NULL,
    subject text NOT NULL,
    product text NOT NULL CHECK (product IN (
        'hub','mail','drive','docs','sheets','slides','pdf','calendar',
        'contacts','notes','photos','rooms','cast','erp','bases'
    )),
    room_id text PRIMARY KEY,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX matrix_storage_rooms_actor
    ON matrix_storage_rooms(tenant,subject,product,created_at DESC);
ALTER TABLE matrix_storage_rooms ENABLE ROW LEVEL SECURITY;
ALTER TABLE matrix_storage_rooms FORCE ROW LEVEL SECURITY;
CREATE POLICY matrix_storage_rooms_tenant ON matrix_storage_rooms
    USING (tenant=current_setting('app.tenant',true))
    WITH CHECK (tenant=current_setting('app.tenant',true));
CREATE POLICY matrix_storage_rooms_maintenance ON matrix_storage_rooms
    USING (current_setting('app.maintenance',true)='on')
    WITH CHECK (current_setting('app.maintenance',true)='on');
CREATE POLICY matrix_storage_rooms_platform_merge ON matrix_storage_rooms
    USING (current_setting('app.merge',true)='on')
    WITH CHECK (current_setting('app.merge',true)='on');
