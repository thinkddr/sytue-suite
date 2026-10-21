-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
DELETE FROM base_permissions WHERE principal_kind = 'group';
ALTER TABLE base_permissions
    DROP CONSTRAINT base_permissions_principal_kind_check,
    ADD CONSTRAINT base_permissions_principal_kind_check CHECK (principal_kind IN ('user', 'tenant')),
    DROP COLUMN principal_label;
