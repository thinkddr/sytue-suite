-- SPDX-License-Identifier: Apache-2.0
-- Cola del borrado de OBJETOS de un tenant purgado (ADR 0054, enmienda del
-- 2026-11-25). Sytue ID borra las filas de la base con sus credenciales de
-- fusión (pg.PurgarTenant) y, cuando la base ya no guarda nada del tenant, deja
-- aquí la orden; el propio servicio, que es quien tiene la clave de su almacén,
-- borra lo que cuelga de `tenants/<slug>/` (y de los prefijos que declare) y lo
-- marca hecho. Sytue ID lee esta tabla para saber cuándo ha terminado.
--
-- La columna se llama `slug` y no `tenant` a propósito: así la tabla queda fuera
-- del inventario de fusión y de la propia purga, que la borraría a la vez que la
-- escribe. Por la misma razón no se mueve en una fusión: una orden de borrado es
-- de la organización que desaparece.
CREATE TABLE IF NOT EXISTS platform_tenant_purges (
    slug              text        PRIMARY KEY
                      CHECK (slug ~ '^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$'),
    requested_at      timestamptz NOT NULL DEFAULT now(),
    -- NULL mientras la base aún guarda filas del tenant (o la purga quedó
    -- retenida): hasta entonces los objetos NO se tocan, porque una fila viva
    -- puede seguir apuntando a ellos.
    db_purged_at      timestamptz,
    objects_purged_at timestamptz,
    attempts          integer     NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    last_attempt_at   timestamptz,
    last_error        text        NOT NULL DEFAULT ''
);

-- Claves sueltas que no cuelgan de ningún prefijo del tenant: un objeto
-- compartido entre organizaciones cuyo contador llegó a cero con la purga (los
-- .eml deduplicados de Mail anteriores al 2026-10-16). Una clave acabada en `/`
-- es un prefijo entero que solo nombraba una fila ya borrada (las webs de Sytue
-- Web, en `shared/sitios/<id>/`). Las anota el gancho de cada base.
CREATE TABLE IF NOT EXISTS platform_tenant_purge_keys (
    slug text NOT NULL REFERENCES platform_tenant_purges (slug) ON DELETE CASCADE,
    key  text NOT NULL CHECK (key <> '' AND octet_length(key) <= 1024),
    PRIMARY KEY (slug, key)
);

CREATE INDEX IF NOT EXISTS platform_tenant_purges_pendientes_idx
    ON platform_tenant_purges (last_attempt_at NULLS FIRST)
    WHERE db_purged_at IS NOT NULL AND objects_purged_at IS NULL;

-- RLS forzada, sin acceso de ninguna transacción de negocio: escribe la purga
-- (app.merge, Sytue ID) y la drena el trabajador del servicio (app.maintenance).
ALTER TABLE platform_tenant_purges     ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform_tenant_purges     FORCE ROW LEVEL SECURITY;
ALTER TABLE platform_tenant_purge_keys ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform_tenant_purge_keys FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS platform_tenant_purges_platform_merge ON platform_tenant_purges;
CREATE POLICY platform_tenant_purges_platform_merge ON platform_tenant_purges
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
DROP POLICY IF EXISTS platform_tenant_purges_maintenance ON platform_tenant_purges;
CREATE POLICY platform_tenant_purges_maintenance ON platform_tenant_purges
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');
DROP POLICY IF EXISTS platform_tenant_purge_keys_platform_merge ON platform_tenant_purge_keys;
CREATE POLICY platform_tenant_purge_keys_platform_merge ON platform_tenant_purge_keys
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
DROP POLICY IF EXISTS platform_tenant_purge_keys_maintenance ON platform_tenant_purge_keys;
CREATE POLICY platform_tenant_purge_keys_maintenance ON platform_tenant_purge_keys
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');
