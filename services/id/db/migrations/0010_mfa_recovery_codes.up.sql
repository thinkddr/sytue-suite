-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Códigos de recuperación de la verificación en dos pasos (TOTP). Se generan al
-- activar el TOTP y se muestran UNA sola vez; solo se persiste el HASH SHA-256 de
-- cada código (nunca el código en claro): defensa ante filtración de la BD. Cada
-- código es de UN SOLO USO — la fila se BORRA al consumirlo en el login (o al
-- desactivar/regenerar el factor). Alta entropía ⇒ SHA-256 basta (igual que los
-- tokens de reseteo); no necesita Argon2.
CREATE TABLE mfa_recovery_codes (
    user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    code_hash  text NOT NULL,                        -- SHA-256 (base64url) del código normalizado
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, code_hash)
);
-- La PK (user_id, code_hash) ya indexa el prefijo user_id: consumir por
-- (user, hash), contar y borrar por user van por Index Scan sin escaneo secuencial
-- a escala (sin índice adicional).
