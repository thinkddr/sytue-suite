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

-- LOS .eml DEDUPLICADOS AL PURGAR UN TENANT. Un DELETE de `messages` por tenant
-- no sabe de `blob_refs` (mig. 0017): los mensajes se irían y el contador de
-- cada .eml compartido se quedaría alto para siempre, con su objeto —el correo
-- de la persona— sin borrar nunca. Este gancho, que la purga llama antes de
-- recorrer las tablas, hace lo que `soltarBlobs` hace al purgar un mensaje:
--
--   * las claves de la organización (`shared/mensajes/<slug>/`, desde el
--     2026-10-16) son solo suyas: se borra su contabilidad y el trabajador barre
--     el prefijo entero (Prefijos en cmd/mail);
--   * las anteriores, compartidas ENTRE organizaciones, bajan una referencia por
--     fila; las que llegan a cero se cierran y su clave va a la orden de borrado.
--     Las que otra organización sigue usando no se tocan.
--
-- Borra él mismo los mensajes, en el mismo bloque: si un guardián los retiene (el
-- almacenamiento privado congelado), se deshace también la cuenta de referencias
-- y la purga de esta base queda retenida (PTP01) con su motivo.
CREATE FUNCTION platform_tenant_purge_prepare(t text) RETURNS text
LANGUAGE plpgsql
AS $$
DECLARE
    mensajes integer := 0;
    cerradas integer := 0;
    propias  integer := 0;
    sin_referencias text[];
BEGIN
    -- Solo dentro de una purga: con su `app.tenant`, una transacción normal de la
    -- aplicación vería sus mensajes y podría borrárselos llamando a esto.
    IF current_setting('app.merge', true) IS DISTINCT FROM 'on' THEN
        RAISE EXCEPTION 'platform_tenant_purge_prepare: solo puede ejecutarse dentro de una purga';
    END IF;
    IF t IS NULL OR t !~ '^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$' THEN
        RAISE EXCEPTION 'platform_tenant_purge_prepare: tenant no válido: %', t;
    END IF;
    BEGIN
        -- Todo en una sentencia: las CTE leen la misma foto, así que las
        -- referencias se cuentan sobre los mensajes de antes de borrarlos.
        WITH refs AS (
            SELECT blob_key AS key, count(*) AS n FROM messages
            WHERE tenant = t AND blob_key LIKE 'shared/%'
              AND blob_key NOT LIKE 'shared/mensajes/' || t || '/%'
            GROUP BY blob_key
        ), borrados AS (
            DELETE FROM messages WHERE tenant = t RETURNING 1
        ), bajadas AS (
            UPDATE blob_refs b SET refs = GREATEST(b.refs - r.n, 0)
              FROM refs r WHERE b.key = r.key
            RETURNING b.key, b.refs
        )
        SELECT (SELECT count(*) FROM borrados),
               coalesce((SELECT array_agg(key) FROM bajadas WHERE refs = 0), '{}')
          INTO mensajes, sin_referencias;

        IF cardinality(sin_referencias) > 0 THEN
            INSERT INTO platform_tenant_purges (slug) VALUES (t) ON CONFLICT (slug) DO NOTHING;
            INSERT INTO platform_tenant_purge_keys (slug, key)
                SELECT t, k FROM unnest(sin_referencias) k ON CONFLICT DO NOTHING;
            DELETE FROM blob_refs WHERE key = ANY (sin_referencias) AND refs = 0;
            GET DIAGNOSTICS cerradas = ROW_COUNT;
        END IF;

        DELETE FROM blob_refs WHERE key LIKE 'shared/mensajes/' || t || '/%';
        GET DIAGNOSTICS propias = ROW_COUNT;
    EXCEPTION WHEN SQLSTATE 'PST01' THEN
        RAISE EXCEPTION 'el correo del tenant está en el almacenamiento privado y su fuente sigue retenida: %', SQLERRM
            USING ERRCODE = 'PTP01';
    END;
    IF mensajes = 0 AND cerradas = 0 AND propias = 0 THEN
        RETURN NULL;
    END IF;
    RETURN format('Mail: %s mensaje(s) borrados; %s .eml compartido(s) entre organizaciones sin más referencias, '
        'y %s de la organización, a borrar del almacén', mensajes, cerradas, propias);
END $$;

COMMENT ON FUNCTION platform_tenant_purge_prepare(text) IS
    'Gancho de la purga de tenant: borra los mensajes y cierra su contabilidad de .eml compartidos. Lo llama pg.PurgarTenant.';
