-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- Lo canjeado se pierde: quien lo tenía vuelve a canjear el enlace.
DROP FUNCTION IF EXISTS docs_anfitrion(uuid, text);
DROP FUNCTION IF EXISTS docs_canjear_enlace(text);
-- Bajo FORCE RLS el DELETE no ve ninguna fila sin un alcance entre organizaciones:
-- se toma el de la fusión (`<tabla>_platform_merge`), solo en esta transacción.
SELECT set_config('app.merge', 'on', true);
DELETE FROM doc_permissions WHERE via_link IS NOT NULL OR externo;
DROP INDEX IF EXISTS doc_permissions_externo_idx;
ALTER TABLE doc_permissions DROP COLUMN IF EXISTS externo, DROP COLUMN IF EXISTS via_link;
