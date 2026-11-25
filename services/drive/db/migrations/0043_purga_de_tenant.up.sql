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

-- LOS FICHEROS DEDUPLICADOS AL PURGAR UN TENANT (mig. 0022). Los de la
-- organización (`shared/drive/<slug>/`, desde el 2026-10-16) son solo suyos y el
-- trabajador barre el prefijo entero (Prefijos en cmd/drive). Los anteriores,
-- `shared/drive/<sha256>`, pueden seguir en uso por otra organización: se anotan
-- como candidatos y el barrido de mantenimiento, que ve todas, los borra pasado
-- su margen si nadie los usa. Es lo mismo que hace `soltarBlob` al purgar una fila.
CREATE FUNCTION platform_tenant_purge_prepare(t text) RETURNS text
LANGUAGE plpgsql
AS $$
DECLARE
    candidatos integer := 0;
BEGIN
    -- Solo dentro de una purga (ver el gancho de la fusión de Mail, mig. 0013).
    IF current_setting('app.merge', true) IS DISTINCT FROM 'on' THEN
        RAISE EXCEPTION 'platform_tenant_purge_prepare: solo puede ejecutarse dentro de una purga';
    END IF;
    IF t IS NULL OR t !~ '^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$' THEN
        RAISE EXCEPTION 'platform_tenant_purge_prepare: tenant no válido: %', t;
    END IF;
    INSERT INTO blobs_candidatos (storage_key)
        SELECT DISTINCT k.storage_key FROM (
            SELECT storage_key FROM files WHERE tenant = t
            UNION ALL
            SELECT storage_key FROM file_versions WHERE tenant = t
        ) k
        WHERE k.storage_key LIKE 'shared/drive/%'
          AND k.storage_key NOT LIKE 'shared/drive/' || t || '/%'
    ON CONFLICT (storage_key) DO NOTHING;
    GET DIAGNOSTICS candidatos = ROW_COUNT;
    IF candidatos = 0 THEN
        RETURN NULL;
    END IF;
    RETURN format('Drive: %s fichero(s) compartido(s) entre organizaciones, candidatos al barrido de huérfanos', candidatos);
END $$;

COMMENT ON FUNCTION platform_tenant_purge_prepare(text) IS
    'Gancho de la purga de tenant: anota los blobs compartidos que el tenant usaba. Lo llama pg.PurgarTenant.';
