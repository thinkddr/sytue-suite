-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Soluciones ALTCHA de la denuncia sin cuenta ya usadas, COMPARTIDAS entre
-- réplicas (ADR 0074). En memoria, una misma solución valía una vez por réplica y
-- Cast corre con dos o más. Lo escribe altcha.EnTabla; es la misma tabla que
-- `captcha_used` de Sytue ID, `base_altcha_used` de Bases y `altcha_used` de Web.
--
-- Sin columna `tenant`: la firma es anónima, así que no entra en la fusión de
-- organizaciones (migcheck no pide política de fusión).
CREATE TABLE altcha_used (
    signature  text PRIMARY KEY,
    expires_at timestamptz NOT NULL
);
CREATE INDEX altcha_used_caducadas ON altcha_used (expires_at);
