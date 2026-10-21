-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Vuelve al esquema anterior. ⚠️ Los tokens EN CLARO no se pueden recuperar de su
-- hash: al revertir, `token` queda NULL y **los enlaces públicos ya repartidos
-- dejan de resolver**. Hay que volver a emitirlos. Es el precio de haber dejado
-- de guardar el secreto, y por eso la reversión no es transparente.

DROP FUNCTION IF EXISTS drive_resolve_share_tenant(text);

ALTER TABLE shares ADD COLUMN IF NOT EXISTS token text;
DROP INDEX IF EXISTS shares_token_hash_uniq;
ALTER TABLE shares DROP COLUMN IF EXISTS token_hash;

CREATE FUNCTION drive_resolve_share_tenant(p_token text)
RETURNS SETOF text
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
BEGIN
    PERFORM set_config('app.resolve_link', 'on', true);
    RETURN QUERY
        SELECT tenant
        FROM shares
        WHERE token = p_token
          AND revoked_at IS NULL
          AND (expires_at IS NULL OR expires_at > now())
        LIMIT 1;
    PERFORM set_config('app.resolve_link', 'off', true);
END;
$$;

REVOKE ALL ON FUNCTION drive_resolve_share_tenant(text) FROM PUBLIC;
