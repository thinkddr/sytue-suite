-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Las extensiones (pgcrypto, pg_trgm) se dejan instaladas a propósito: son
-- globales de la base y otros objetos pueden depender de ellas.

DROP TABLE IF EXISTS modulos_activos;
DROP TABLE IF EXISTS sequences;
DROP TABLE IF EXISTS move_lines;
DROP TABLE IF EXISTS moves;
DROP TABLE IF EXISTS records;

DROP FUNCTION IF EXISTS syerp_apunte_inmutable();
DROP FUNCTION IF EXISTS syerp_asiento_inmutable();
DROP FUNCTION IF EXISTS syerp_asiento_cuadrado();
