-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Revierte 0003: elimina la política aditiva del resolutor y restaura la función
-- a su forma de 0002 (RETURNS text escalar, sin salto de RLS acotado).

DROP POLICY IF EXISTS shares_public_link_resolver ON shares;

DROP FUNCTION IF EXISTS drive_resolve_share_tenant(text);
CREATE FUNCTION drive_resolve_share_tenant(p_token text)
RETURNS text
LANGUAGE sql
SECURITY DEFINER
SET search_path = pg_catalog, public
STABLE
AS $$
    SELECT tenant
    FROM shares
    WHERE token = p_token
      AND revoked_at IS NULL
      AND (expires_at IS NULL OR expires_at > now())
    LIMIT 1
$$;

REVOKE ALL ON FUNCTION drive_resolve_share_tenant(text) FROM PUBLIC;
