-- SPDX-License-Identifier: Apache-2.0
-- Invalidate outstanding cross-app handoffs before reverting to Rooms-only.
BEGIN;
SELECT set_config('app.merge','on',true);
DELETE FROM matrix_native_consents;
ALTER TABLE matrix_native_consents DROP COLUMN product;
COMMIT;
