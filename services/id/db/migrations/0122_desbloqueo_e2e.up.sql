-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- El desbloqueo del cifrado con el inicio de sesión de Sytue ID (ADR 0084).
--
-- Cada persona tiene una CLAVE DE DESBLOQUEO que nace en un navegador y que este
-- servicio solo guarda dentro de sobres: uno de contraseña (Argon2id de la
-- contraseña, en el navegador) y uno por passkey (su salida PRF, en el navegador).
-- Ni la clave, ni la contraseña derivada, ni la salida PRF llegan aquí: un sobre no
-- se abre con nada que tenga el servidor. `hint` es la huella pública de la clave:
-- todos los sobres de una persona llevan la misma, y una que no coincide se rechaza
-- salvo al empezar de cero. La sal de quien aún no tiene sobre no se guarda: se
-- deriva de la KEK del servicio (como las claves HMAC de reseteo y consentimiento),
-- igual que la de una dirección sin cuenta, para no decir cuál es cuál.
CREATE TABLE e2e_unlock_envelopes (
    user_id       uuid        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind          text        NOT NULL CHECK (kind IN ('password','passkey')),
    credential_id text        NOT NULL DEFAULT '',
    hint          text        NOT NULL CHECK (hint <> ''),
    envelope      jsonb       NOT NULL,
    updated_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, kind, credential_id),
    CHECK ((kind = 'password') = (credential_id = ''))
);

