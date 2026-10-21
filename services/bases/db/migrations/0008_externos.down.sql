-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
DROP POLICY IF EXISTS base_permissions_externo ON base_permissions;
DROP INDEX IF EXISTS base_permissions_externos;
DELETE FROM base_permissions WHERE principal_kind = 'external';
ALTER TABLE base_permissions
    DROP CONSTRAINT base_permissions_principal_kind_check,
    ADD CONSTRAINT base_permissions_principal_kind_check CHECK (principal_kind IN ('user', 'group', 'tenant'));
