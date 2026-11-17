-- SPDX-License-Identifier: Apache-2.0
-- Old code must never consume a grant whose receiving app it cannot bind.
BEGIN;
SELECT set_config('app.maintenance','on',true),set_config('app.matrix_native_consumer_contract','1',true);
LOCK TABLE matrix_native_consents IN ACCESS EXCLUSIVE MODE;
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM matrix_native_consents WHERE native_consumer <> '') THEN
        RAISE EXCEPTION 'native consumer grants must expire or be retired before rollback';
    END IF;
END $$;
DROP TRIGGER matrix_native_consumers_retire_identity ON matrix_identity_access;
DROP TRIGGER matrix_native_consumers_retire_tenant ON matrix_tenant_access;
DROP FUNCTION matrix_native_consumers_retire();
DROP POLICY matrix_native_consents_consumer_contract ON matrix_native_consents;
ALTER TABLE matrix_native_consents DROP COLUMN native_consumer;
COMMIT;
