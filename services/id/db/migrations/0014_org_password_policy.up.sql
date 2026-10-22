-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Política de contraseñas POR-ORG. Dos columnas en `organizations` (no jsonb: son
-- dos escalares que se leen en cada alta/cambio de contraseña; una columna tipada
-- es más barata de leer y validar que descomponer jsonb). Default sensato que
-- hereda toda org existente y coincide con domain.DefaultPasswordPolicy:
--   - password_min_length = 12  (longitud sobre composición, NIST 800-63B)
--   - password_require_complexity = false
--
-- El enforcement vive en domain.ValidatePassword (choke point único); esta tabla
-- solo persiste la política. Aislamiento por org_id como el resto del servicio
-- (sin RLS; la FUENTE acota por columna).
ALTER TABLE organizations
	ADD COLUMN password_min_length int NOT NULL DEFAULT 12,
	ADD COLUMN password_require_complexity boolean NOT NULL DEFAULT false;
