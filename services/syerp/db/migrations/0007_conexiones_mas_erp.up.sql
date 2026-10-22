-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- Microsoft Dynamics 365 Business Central y Holded entran como proveedores de
-- conexión: importan por la misma pasarela que Salesforce y Odoo.
ALTER TABLE conexiones DROP CONSTRAINT conexiones_proveedor_check;
ALTER TABLE conexiones ADD CONSTRAINT conexiones_proveedor_check
    CHECK (proveedor IN ('stripe', 'salesforce', 'odoo', 'business-central', 'holded'));
