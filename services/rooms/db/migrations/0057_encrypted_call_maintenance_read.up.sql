-- SPDX-License-Identifier: Apache-2.0
-- The revoker and expiry worker enumerate only document calls or existing
-- encrypted epochs across organizations. This policy grants SELECT only; every
-- callback mutation must still SET LOCAL app.tenant for the affected room.
CREATE POLICY rooms_encrypted_call_maintenance_read ON rooms FOR SELECT
 USING(current_setting('app.maintenance',true)='on' AND
 (metadata->'source'->>'kind'='document' OR EXISTS(
 SELECT 1 FROM matrix_call_epochs e WHERE e.tenant=rooms.tenant AND e.call_id=rooms.id)));
