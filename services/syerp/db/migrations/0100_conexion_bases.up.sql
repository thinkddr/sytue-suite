-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- Sytue Bases entra como proveedor de conexión: una VISTA de una base, de solo lectura,
-- como fuente de los informes (ADR 0068). El secreto es el token de la conexión (`sbc_…`),
-- sellado como los demás; `config` guarda las columnas y la procedencia de la última lectura.
ALTER TABLE conexiones DROP CONSTRAINT conexiones_proveedor_check;
ALTER TABLE conexiones ADD CONSTRAINT conexiones_proveedor_check
    CHECK (proveedor IN ('stripe', 'salesforce', 'odoo', 'business-central', 'holded', 'bases'));
