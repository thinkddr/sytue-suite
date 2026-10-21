-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Tokens de verificación de email (alta sin invitación / cambio de email). Mismo
-- modelo de token que password_reset_tokens (mig 0009): solo se persiste el HASH
-- SHA-256 del token (nunca el token en claro), el token es además FIRMADO (HMAC
-- del servidor) y de un solo uso — se BORRA al consumirlo (DELETE ... RETURNING
-- atómico) — con caducidad por expires_at. La fila lleva la DIRECCIÓN que se
-- verifica: al consumir solo se marca email_verified si sigue coincidiendo con el
-- email actual del usuario (un token queda ligado a la dirección para la que se
-- emitió; si el correo cambió entre tanto, el enlace deja de aplicar). La columna
-- users.email_verified ya existe (mig 0001): esto NO añade columna, solo la tabla
-- de tokens de un solo uso.
CREATE TABLE email_verification_tokens (
    token_hash  text PRIMARY KEY,                             -- SHA-256 (base64url) del token
    user_id     uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    email       text NOT NULL,                                -- dirección que se verifica (normalizada a minúsculas)
    expires_at  timestamptz NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);

-- user_idx: invalidar en bloque los tokens previos de un usuario al emitir uno
-- nuevo (solo el último enlace vale) — evita N+1 y Seq Scan a escala.
CREATE INDEX email_verification_tokens_user_idx ON email_verification_tokens (user_id);
-- expires_idx: purga periódica de los caducados por Index Scan (CleanupExpired).
CREATE INDEX email_verification_tokens_expires_idx ON email_verification_tokens (expires_at);
