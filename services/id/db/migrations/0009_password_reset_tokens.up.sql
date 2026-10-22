-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Tokens de reseteo de contraseña ("olvidé mi contraseña"). Solo se persiste el
-- HASH SHA-256 del token (nunca el token en claro): defensa ante filtración de la
-- BD. El token es además FIRMADO (HMAC del servidor) y de un solo uso: se BORRA al
-- consumirlo (DELETE ... RETURNING atómico), con caducidad corta por expires_at.
CREATE TABLE password_reset_tokens (
    token_hash  text PRIMARY KEY,                             -- SHA-256 (base64url) del token
    user_id     uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    expires_at  timestamptz NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);

-- user_idx: invalidar en bloque los tokens previos de un usuario al emitir uno
-- nuevo (solo el último enlace vale) — evita N+1 y Seq Scan a escala.
CREATE INDEX password_reset_tokens_user_idx ON password_reset_tokens (user_id);
-- expires_idx: purga periódica de los caducados por Index Scan (CleanupExpired).
CREATE INDEX password_reset_tokens_expires_idx ON password_reset_tokens (expires_at);
