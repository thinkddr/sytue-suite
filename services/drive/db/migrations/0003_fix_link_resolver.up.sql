-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Corrige la resolución de enlaces públicos (drive_resolve_share_tenant), rota en
-- dos frentes:
--
-- (1) RLS: `shares` tiene FORCE ROW LEVEL SECURITY y el rol de la app (dueño de la
--     tabla y de la función) NO tiene BYPASSRLS, así que SECURITY DEFINER por sí
--     solo NO salta la RLS: con el visitante anónimo (sin `app.tenant`) la política
--     de aislamiento filtra TODAS las filas y la función devolvía NULL para
--     CUALQUIER token —incluso uno válido y vivo—. Resultado: GET /public/{token}
--     respondía 500/no encontrado incluso para enlaces legítimos.
--
-- (2) Escaneo NULL: al ser `RETURNS text` (escalar), un token inexistente devolvía
--     una fila con NULL, que el store escanea en un `string` no-puntero →
--     "cannot scan NULL into *string" → 500 en vez de 404 (mismo fallo que arregló
--     Sytue Meet en su 0002).
--
-- Arreglo, sin tocar el binario Go (solo esquema):
--   · La función pasa a `RETURNS SETOF text`: 0 filas si no hay match (el store lo
--     mapea a pgx.ErrNoRows → 404 limpio), 1 fila con el tenant si lo hay.
--   · Salto de RLS ACOTADO a la ejecución de la función: fija el GUC de sesión
--     `app.resolve_link='on'` (local a la transacción) SOLO mientras lee `shares`
--     y lo restaura a 'off' acto seguido. Una política aditiva SELECT-only habilita
--     la lectura únicamente con ese GUC activo. Fuera de la función el aislamiento
--     por `app.tenant` sigue intacto (la función está REVOKE de PUBLIC; el token de
--     alta entropía sigue siendo el secreto). El re-lectura posterior del store,
--     tras fijar `app.tenant`, opera bajo la RLS normal.
--
-- Idempotente: se puede reaplicar sin efectos (CREATE OR REPLACE / IF EXISTS).

-- (1)+(2) Reescritura de la función. Cambiar el tipo de retorno (text → SETOF text)
-- obliga a recrearla.
DROP FUNCTION IF EXISTS drive_resolve_share_tenant(text);
CREATE FUNCTION drive_resolve_share_tenant(p_token text)
RETURNS SETOF text
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
BEGIN
    -- Salto de RLS acotado: activo SOLO durante la lectura de `shares` de abajo.
    PERFORM set_config('app.resolve_link', 'on', true);
    RETURN QUERY
        SELECT tenant
        FROM shares
        WHERE token = p_token
          AND revoked_at IS NULL
          AND (expires_at IS NULL OR expires_at > now())
        LIMIT 1;
    -- Restaura el aislamiento normal para el resto de la transacción.
    PERFORM set_config('app.resolve_link', 'off', true);
END;
$$;

REVOKE ALL ON FUNCTION drive_resolve_share_tenant(text) FROM PUBLIC;

-- Política aditiva (permisiva, se combina con OR): SELECT permitido SOLO cuando el
-- GUC de salto acotado está activo, que únicamente ocurre dentro de la función de
-- arriba. No relaja shares_tenant_isolation: las lecturas/escrituras normales de la
-- app siguen filtradas por `app.tenant`.
DROP POLICY IF EXISTS shares_public_link_resolver ON shares;
CREATE POLICY shares_public_link_resolver ON shares
    FOR SELECT
    USING (current_setting('app.resolve_link', true) = 'on');
