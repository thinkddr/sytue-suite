-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Sytue ID — rotación programada de JWKS y cifrado en reposo de la clave privada.
-- Cierra los hallazgos F9 (private_pem cifrado en reposo) y F10 (rotación con
-- retiro tras una ventana >= vida del access token) de docs/security-audit-id.md.

-- demoted_at: instante en que la clave pasó de 'active' a 'next'. El job de
-- rotación retira ('retired') una clave 'next' cuando now() - demoted_at supera
-- la ventana de gracia (>= vida del access token), garantizando que ningún token
-- en vuelo firmado por esa clave quede sin poder validarse.
ALTER TABLE signing_keys ADD COLUMN IF NOT EXISTS demoted_at timestamptz;

-- Índice parcial para el barrido de retiro (solo claves candidatas 'next').
CREATE INDEX IF NOT EXISTS signing_keys_next_demoted_idx
    ON signing_keys (demoted_at) WHERE status = 'next';

-- private_pem se cifra en reposo a nivel de aplicación (AES-256-GCM con KEK del
-- Secret Manager/KMS): el valor almacenado es el sobre "encv1:base64(...)". La
-- columna sigue siendo text; el cambio es de contenido, no de tipo. Las claves
-- preexistentes en claro se leen tal cual hasta su rotación (migración sin
-- downtime). Se documenta aquí para el operador.
COMMENT ON COLUMN signing_keys.private_pem IS
    'Clave privada de firma cifrada en reposo (AES-256-GCM, sobre "encv1:..."). Nunca en claro en prod (F9).';

-- Continuación de login MFA (F8): tras verificar la contraseña (paso 1), se emite
-- un token de continuación de un solo uso, corta vida y ligado a IP/UA. El paso 2
-- (TOTP/WebAuthn) lo consume y solo entonces se crea la sesión con amr elevado.
-- Solo se persiste el HASH del token (SHA-256), nunca el token en claro.
CREATE TABLE mfa_continuations (
    token_hash  text PRIMARY KEY,                          -- SHA-256 del token de continuación
    user_id     uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    org_id      uuid REFERENCES organizations (id) ON DELETE SET NULL,
    return_to   text NOT NULL DEFAULT '',                  -- destino tras completar el 2FA (ruta interna)
    ip          inet,                                      -- binding de red del paso 1
    user_agent  text NOT NULL DEFAULT '',                  -- binding de cliente del paso 1
    expires_at  timestamptz NOT NULL,
    consumed_at timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX mfa_continuations_expires_idx ON mfa_continuations (expires_at);

-- El secreto TOTP se cifra en reposo (AES-256-GCM) igual que la clave de firma.
COMMENT ON COLUMN totp_secrets.secret IS
    'Secreto TOTP cifrado en reposo (AES-256-GCM, sobre "encv1:..."). Nunca en claro en prod.';
