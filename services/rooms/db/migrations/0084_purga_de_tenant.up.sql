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

-- LA CONFIANZA QUE OTRAS ORGANIZACIONES TENÍAN EN LA PURGADA (mig 0083). Sus filas
-- de `politica_contacto_externo` las borra el recorrido por tenant como las demás;
-- lo que no ve es su slug en la lista `confianza` de OTRA organización. Se quita
-- ahí: un slug que queda libre lo puede registrar mañana otra organización, y no
-- debe heredar una confianza que se dio a la que ya no existe.
CREATE FUNCTION platform_tenant_purge_prepare(t text) RETURNS text
LANGUAGE plpgsql
AS $$
DECLARE
    politicas integer := 0;
BEGIN
    -- Solo dentro de una purga (como el gancho de la fusión, mig 0083).
    IF current_setting('app.merge', true) IS DISTINCT FROM 'on' THEN
        RAISE EXCEPTION 'platform_tenant_purge_prepare: solo puede ejecutarse dentro de una purga';
    END IF;
    IF t IS NULL OR t !~ '^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$' THEN
        RAISE EXCEPTION 'platform_tenant_purge_prepare: tenant no válido: %', t;
    END IF;
    UPDATE politica_contacto_externo
        SET confianza = array_remove(confianza, t)
        WHERE tenant <> t AND t = ANY (confianza);
    GET DIAGNOSTICS politicas = ROW_COUNT;
    IF politicas = 0 THEN
        RETURN NULL;
    END IF;
    RETURN format('Rooms: retirada de la lista de confianza de %s organización(es)', politicas);
END $$;

COMMENT ON FUNCTION platform_tenant_purge_prepare(text) IS
    'Gancho de la purga de tenant: quita el slug purgado de la confianza de las demás organizaciones. Lo llama pg.PurgarTenant.';
