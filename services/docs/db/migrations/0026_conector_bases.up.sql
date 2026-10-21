-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Los orígenes de datos de Calcus admiten dos tipos más: `gateway` (Sytue Integration
-- Gateway), que el código ya ofrecía pero la restricción de la 0003 rechazaba al guardar,
-- y `bases` (Sytue Bases, de solo lectura). Solo se ENSANCHA la lista: nada de lo que el
-- binario anterior escribe deja de valer.
ALTER TABLE data_connections DROP CONSTRAINT IF EXISTS data_connections_kind_chk;
ALTER TABLE data_connections ADD CONSTRAINT data_connections_kind_chk
    CHECK (kind IN ('rest', 'postgres', 'gateway', 'bases'));
