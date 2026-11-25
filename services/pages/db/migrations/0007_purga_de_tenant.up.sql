-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Cola del borrado de OBJETOS de un tenant purgado (ADR 0054, enmienda del
-- 2026-11-25). Sytue ID borra las filas de esta base con sus credenciales de
-- fusión (pg.PurgarTenant) y, cuando ya no queda nada del tenant, deja aquí la
-- orden; el trabajador de purga de este servicio (platform/pkg/purgatenant), que
-- es quien tiene la clave del almacén, borra sus objetos y la marca hecha.
--
-- El SQL es el común de platform/pkg/pg/tenant_purge_v1.sql:
-- sytue:tenant-purge-v1
--
-- Si db.Migrations no expandiera el marcador, esto falla en vez de dejar el
-- servicio sin cola en silencio.
SELECT 'platform_tenant_purges'::regclass, 'platform_tenant_purge_keys'::regclass;

-- LAS WEBS PUBLICADAS AL PURGAR UN TENANT. Sus ficheros no cuelgan del prefijo
-- de la organización sino de `shared/sitios/<id>/` (internal/sitios), y el id
-- solo lo sabe la fila de `sites`, que la purga va a borrar. El gancho deja cada
-- prefijo en la orden ANTES de que desaparezca la fila.
CREATE FUNCTION platform_tenant_purge_prepare(t text) RETURNS text
LANGUAGE plpgsql
AS $$
DECLARE
    webs integer := 0;
BEGIN
    -- Solo dentro de una purga (ver el gancho de la fusión de Mail, mig. 0013).
    IF current_setting('app.merge', true) IS DISTINCT FROM 'on' THEN
        RAISE EXCEPTION 'platform_tenant_purge_prepare: solo puede ejecutarse dentro de una purga';
    END IF;
    IF t IS NULL OR t !~ '^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$' THEN
        RAISE EXCEPTION 'platform_tenant_purge_prepare: tenant no válido: %', t;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM sites WHERE tenant = t) THEN
        RETURN NULL;
    END IF;
    INSERT INTO platform_tenant_purges (slug) VALUES (t) ON CONFLICT (slug) DO NOTHING;
    INSERT INTO platform_tenant_purge_keys (slug, key)
        SELECT t, 'shared/sitios/' || id::text || '/' FROM sites WHERE tenant = t
    ON CONFLICT DO NOTHING;
    GET DIAGNOSTICS webs = ROW_COUNT;
    RETURN format('Web: %s sitio(s) cuyos ficheros publicados se borran del almacén', webs);
END $$;

COMMENT ON FUNCTION platform_tenant_purge_prepare(text) IS
    'Gancho de la purga de tenant: anota los prefijos de las webs publicadas. Lo llama pg.PurgarTenant.';
