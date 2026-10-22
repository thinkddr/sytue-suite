-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

DROP TRIGGER IF EXISTS records_factura_emitida ON records;
DROP FUNCTION IF EXISTS verifactu_factura_emitida();
DROP TABLE IF EXISTS verifactu_registros;
DROP FUNCTION IF EXISTS verifactu_registro_inalterable();
ALTER TABLE organizacion
    DROP COLUMN IF EXISTS verifactu_proximo_envio,
    DROP COLUMN IF EXISTS verifactu_instalacion,
    DROP COLUMN IF EXISTS verifactu_desde;
