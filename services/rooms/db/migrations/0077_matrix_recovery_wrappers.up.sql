-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- La clave de recuperación de Matrix, ENVUELTA en el dispositivo (ADR 0082). El
-- servicio guarda solo texto cifrado y parámetros públicos: la clave que lo abre
-- se deriva de la contraseña o de la passkey de la persona y nunca llega aquí.
-- Ni la base, ni la KEK, ni Sytue ID bastan para leerla.
--
-- Una fila por envoltorio de una CUENTA de Matrix: organizacion y product vacíos son
-- `@sytue_<sub>` (Rooms y el almacenamiento heredado, una sola cuenta en todas
-- sus organizaciones); con valor, la cuenta aislada de ese producto en esa
-- organización. Una contraseña por cuenta (credential_id vacío) y una fila por
-- passkey. Solo se ve en mantenimiento, tras comprobar que quien pide es esa persona.
CREATE TABLE matrix_recovery_wrappers (
    id            uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    subject       text        NOT NULL,
    organizacion  text        NOT NULL DEFAULT '',
    product       text        NOT NULL DEFAULT '',
    kind          text        NOT NULL CHECK (kind IN ('password','passkey')),
    credential_id text        NOT NULL DEFAULT '',
    key_id        text        NOT NULL CHECK (key_id <> '' AND length(key_id) <= 128),
    wrapper       jsonb       NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (subject, organizacion, product, kind, credential_id),
    CHECK ((organizacion = '') = (product = '')),
    CHECK ((kind = 'password') = (credential_id = ''))
);
ALTER TABLE matrix_recovery_wrappers ENABLE ROW LEVEL SECURITY;
ALTER TABLE matrix_recovery_wrappers FORCE ROW LEVEL SECURITY;
CREATE POLICY matrix_recovery_wrappers_maintenance ON matrix_recovery_wrappers
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');
-- La columna NO se llama `tenant` a propósito: no son datos de la organización
-- que una fusión mueva ni que la admisión de scope deba vallar. La mayoría de
-- filas son de la cuenta personal (`@sytue_<sub>`, sin organización), que esa
-- guarda rechazaría; y una fila de producto queda atada por su AAD a la cuenta de
-- Matrix de esa organización: tras una fusión es texto cifrado huérfano, inútil
-- para cualquiera, y la cuenta nueva pide envolver de nuevo.
