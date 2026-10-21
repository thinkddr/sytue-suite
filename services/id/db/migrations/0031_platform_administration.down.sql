-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- Las organizaciones fusionadas vuelven a 'suspended': el CHECK original no
-- admite 'merged' y dejarlas en un valor prohibido impediría añadir la propia
-- constraint. No se deshace la fusión de datos, que es irreversible; solo se
-- retira el bookkeeping.
UPDATE organizations SET status = 'suspended' WHERE status = 'merged';

ALTER TABLE organizations DROP CONSTRAINT IF EXISTS organizations_merge_coherente;
ALTER TABLE organizations DROP CONSTRAINT IF EXISTS organizations_status_check;
ALTER TABLE organizations ADD CONSTRAINT organizations_status_check
    CHECK (status IN ('active', 'suspended', 'deleted'));
ALTER TABLE organizations
    DROP COLUMN IF EXISTS merged_at,
    DROP COLUMN IF EXISTS merged_into_org_id;

DROP TRIGGER IF EXISTS org_merges_set_updated_at ON org_merges;
DROP TABLE IF EXISTS org_merges;

DROP TRIGGER IF EXISTS seat_grants_set_updated_at ON seat_grants;
DROP TABLE IF EXISTS seat_grants;

DROP TRIGGER IF EXISTS platform_admins_set_updated_at ON platform_admins;
DROP TABLE IF EXISTS platform_admins;
