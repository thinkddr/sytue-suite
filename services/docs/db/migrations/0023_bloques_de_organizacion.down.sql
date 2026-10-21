-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- Bajar esto convierte los bloques de la organización en plantillas corrientes.
ALTER TABLE doc_org_templates DROP COLUMN IF EXISTS es_bloque;
