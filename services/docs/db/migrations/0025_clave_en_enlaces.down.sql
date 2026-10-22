-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- Bajar esto deja los enlaces SIN contraseña: quien tenga el token vuelve a entrar sin
-- ella. Revocar antes los enlaces protegidos si eso importa.
DROP FUNCTION IF EXISTS docs_enlace_clave(text);
ALTER TABLE doc_share_links DROP COLUMN IF EXISTS password_hash;
