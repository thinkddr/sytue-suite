-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- La clave de recuperación del cifrado de Matrix, custodiada para la persona
-- (ADR 0081). Va por `subject` y no por organización porque la cuenta de Matrix
-- es una sola (`@sytue_<sub>`) en todas sus organizaciones. Se guarda cifrada con
-- la KEK del servicio (`encv1:`), y solo se lee en transacciones de mantenimiento
-- que el servicio abre tras comprobar que quien pide es esa persona.
CREATE TABLE matrix_recovery_keys (
    subject    text        PRIMARY KEY,
    sealed     text        NOT NULL CHECK (sealed LIKE 'encv1:%'),
    updated_at timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE matrix_recovery_keys ENABLE ROW LEVEL SECURITY;
ALTER TABLE matrix_recovery_keys FORCE ROW LEVEL SECURITY;
CREATE POLICY matrix_recovery_keys_maintenance ON matrix_recovery_keys
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');
