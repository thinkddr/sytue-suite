-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Liga el código de autorización a la sesión SSO que lo originó, para propagar
-- SessionID a los refresh tokens emitidos en el canje y así permitir la
-- revocación en cascada al cerrar la sesión (logout / RevokeSession).

ALTER TABLE auth_codes
    ADD COLUMN IF NOT EXISTS session_id uuid REFERENCES sessions (id) ON DELETE SET NULL;

-- Índice para revocar los refresh tokens por sesión en el logout.
CREATE INDEX IF NOT EXISTS refresh_tokens_session_idx
    ON refresh_tokens (session_id) WHERE session_id IS NOT NULL;
