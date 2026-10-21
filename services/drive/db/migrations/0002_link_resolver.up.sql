-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Resolución de enlaces públicos (kind='link') por token. El visitante anónimo
-- no porta tenant, por lo que la RLS por `app.tenant` bloquearía la lectura. El
-- token es un secreto inadivinable (alta entropía), así que exponemos una
-- función SECURITY DEFINER acotada que SOLO devuelve el tenant de un token vivo
-- y no expirado. La app fija después `app.tenant` y vuelve a leer bajo RLS.

CREATE OR REPLACE FUNCTION drive_resolve_share_tenant(p_token text)
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
