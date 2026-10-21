-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
-- La vuelta atrás solo retira las columnas cifradas. `ip` no se toca porque el
-- `up` tampoco la borró: lo cifrado no se puede convertir a claro sin las
-- cookies de los usuarios, que no están aquí.
BEGIN;
ALTER TABLE sessions DROP COLUMN IF EXISTS ip_cifrada;
ALTER TABLE sessions DROP COLUMN IF EXISTS lugar_cifrado;
COMMIT;
