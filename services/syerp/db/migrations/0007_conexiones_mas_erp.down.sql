-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

DELETE FROM conexiones WHERE proveedor IN ('business-central', 'holded');
ALTER TABLE conexiones DROP CONSTRAINT conexiones_proveedor_check;
ALTER TABLE conexiones ADD CONSTRAINT conexiones_proveedor_check
    CHECK (proveedor IN ('stripe', 'salesforce', 'odoo'));
