-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Móvil verificado por SMS y correo de recuperación (ADR 0041).
--
-- El alta autoservicio exige un móvil y un código de un solo uso enviado por SMS.
-- Una vez dentro, se entra con cualquiera de sus cuatro identificadores —@usuario,
-- dirección de Sytue, correo de recuperación o móvil— y la contraseña es siempre
-- la misma, la de la cuenta.
--
-- Expand puro: las dos columnas nacen anulables. Las cuentas que ya existen no
-- tienen móvil y el correo de recuperación es opcional para todas.

ALTER TABLE users ADD COLUMN phone text;
ALTER TABLE users ADD COLUMN recovery_email text;

-- Únicos porque son identificadores de acceso: dos cuentas con el mismo móvil
-- harían que «entrar con el móvil» no supiera en cuál. El móvil va en E.164, así
-- que no hay mayúsculas que plegar; el correo sí.
CREATE UNIQUE INDEX users_phone_idx ON users (phone);
CREATE UNIQUE INDEX users_recovery_email_idx ON users (lower(recovery_email));

-- Altas a medio hacer: lo que la persona rellenó y el hash del código que se le
-- mandó. La cuenta NO existe hasta que el código se confirma, así que aquí no hay
-- user_id. La contraseña ya va en Argon2id, como en users.
CREATE TABLE signup_phone_codes (
    id            uuid PRIMARY KEY,
    phone         text NOT NULL,
    username      text NOT NULL,
    name          text NOT NULL,
    password_hash text NOT NULL,
    code_hash     text NOT NULL,
    attempts      int NOT NULL DEFAULT 0,
    sent_at       timestamptz NOT NULL DEFAULT now(),
    expires_at    timestamptz NOT NULL
);
CREATE INDEX signup_phone_codes_expires_idx ON signup_phone_codes (expires_at);

-- Para qué se emitió cada enlace de verificación: el correo de acceso (lo de
-- siempre) o el de recuperación que se pide en «Mi cuenta». Sin esto, confirmar
-- un enlace viejo del correo de acceso podía acabar tomado como recuperación.
ALTER TABLE email_verification_tokens ADD COLUMN purpose text NOT NULL DEFAULT 'login';
