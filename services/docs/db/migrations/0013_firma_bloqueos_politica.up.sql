-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- Peticiones de firma de fuera: lo que decide quien las recibe y lo que decide su
-- organización (docs/decisions.md#0059).
--
-- · `silenciada`: la petición existe para quien la pidió —se «envió», y la ve en su
--   lista como cualquier otra— pero no llega: ni aviso, ni lista, ni se abre. Es lo
--   que pasa a una petición de alguien bloqueado, o a una que ninguna organización de
--   quien firma admite. Quien pide no puede distinguirla de una que no se ha abierto.
ALTER TABLE pdf_peticiones_firma
    ADD COLUMN silenciada boolean NOT NULL DEFAULT false,
    ADD CONSTRAINT pdf_peticiones_firma_silenciada_chk CHECK (externo OR NOT silenciada);

-- Los bloqueos son de la PERSONA, no de una organización: valen desde cualquiera de
-- las suyas y contra quien pide desde cualquiera de las suyas. `subject` es quien
-- bloquea (el `sub` de su token; con esa columna «Borrar mi cuenta» se lleva los suyos,
-- ADR 0054) y `tenant` solo dice desde qué organización lo hizo: no limita nada. Nadie
-- los lee directamente: sin política propia, la RLS no deja ver ninguna fila a la
-- aplicación, y todo pasa por las funciones de abajo.
CREATE TABLE pdf_firma_bloqueos (
    subject   text        NOT NULL,
    bloqueado text        NOT NULL,
    tenant    text        NOT NULL,
    -- Cómo se enseña en SU lista de bloqueados: el nombre que ya veía en la petición.
    nombre    text        NOT NULL DEFAULT '',
    creado    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (subject, bloqueado),
    CONSTRAINT pdf_firma_bloqueos_nombre_chk CHECK (char_length(nombre) <= 200),
    CONSTRAINT pdf_firma_bloqueos_distintos_chk CHECK (subject <> bloqueado)
);
ALTER TABLE pdf_firma_bloqueos ENABLE ROW LEVEL SECURITY;
ALTER TABLE pdf_firma_bloqueos FORCE ROW LEVEL SECURITY;
CREATE POLICY pdf_firma_bloqueos_platform_merge ON pdf_firma_bloqueos
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');

-- LAS FUNCIONES DE ESTA BASE QUE CRUZAN ORGANIZACIONES Y LA RLS.
--
-- En producción la aplicación y las migraciones usan el MISMO rol, que es dueño de
-- tablas y funciones y NO tiene BYPASSRLS; con FORCE ROW LEVEL SECURITY, un SECURITY
-- DEFINER por sí solo NO salta la RLS (lo encontró Drive en su 0003). En los tests el
-- dueño es superusuario y lo tapaba. La salida es la de Drive: cada función que cruza
-- organizaciones lleva `SET app.docs_definer = 'on'` —vale mientras se ejecuta y se
-- restaura sola al salir, también si falla— y una política aditiva, por tabla y por
-- orden, deja hacer lo que esas funciones hacen solo con ese valor puesto. Fuera de
-- ellas el aislamiento por `app.tenant` sigue intacto, y la aplicación no fija nunca esa
-- variable (como `app.merge`, que solo fija la fusión).
CREATE POLICY documents_docs_definer ON documents FOR SELECT
    USING (current_setting('app.docs_definer', true) = 'on');
CREATE POLICY doc_share_links_docs_definer ON doc_share_links FOR SELECT
    USING (current_setting('app.docs_definer', true) = 'on');
CREATE POLICY doc_permissions_docs_definer ON doc_permissions FOR SELECT
    USING (current_setting('app.docs_definer', true) = 'on');
CREATE POLICY pdf_peticiones_firma_docs_definer_ver ON pdf_peticiones_firma FOR SELECT
    USING (current_setting('app.docs_definer', true) = 'on');
CREATE POLICY pdf_peticiones_firma_docs_definer_silenciar ON pdf_peticiones_firma FOR UPDATE
    USING (current_setting('app.docs_definer', true) = 'on')
    WITH CHECK (current_setting('app.docs_definer', true) = 'on');
CREATE POLICY pdf_firma_bloqueos_docs_definer ON pdf_firma_bloqueos
    USING (current_setting('app.docs_definer', true) = 'on')
    WITH CHECK (current_setting('app.docs_definer', true) = 'on');

-- Las de antes, con la variable puesta. Sin esto, en producción no veían nada: el
-- enlace público de un documento, el canje entre organizaciones (ADR 0052) y las
-- peticiones de firma de fuera (ADR 0057).
ALTER FUNCTION docs_resolve_share(text) SET app.docs_definer = 'on';
ALTER FUNCTION docs_canjear_enlace(text) SET app.docs_definer = 'on';
ALTER FUNCTION docs_anfitrion(uuid, text) SET app.docs_definer = 'on';
ALTER FUNCTION docs_de_fuera(text, text, text, text, boolean, text, timestamptz, uuid, int) SET app.docs_definer = 'on';

-- Bloquear: apunta el bloqueo (hasta 500 por persona) y silencia lo que ya estaba
-- pendiente de esa persona hacia quien bloquea, en todas las organizaciones.
-- Devuelve 0 si lo apuntó, 1 si ya tiene demasiados y 2 si no tiene sentido.
CREATE FUNCTION docs_firma_bloquear(p_quien text, p_bloqueado text, p_nombre text, p_tenant text)
RETURNS int
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, public
SET app.docs_definer = 'on'
AS $$
BEGIN
    IF coalesce(p_quien, '') = '' OR coalesce(p_bloqueado, '') = '' OR p_quien = p_bloqueado THEN
        RETURN 2;
    END IF;
    -- El tope y el alta, de uno en uno por persona.
    PERFORM pg_advisory_xact_lock(hashtext('pdf-firma-bloqueos:' || p_quien));
    IF NOT EXISTS (SELECT 1 FROM pdf_firma_bloqueos WHERE subject = p_quien AND bloqueado = p_bloqueado)
       AND (SELECT count(*) FROM pdf_firma_bloqueos WHERE subject = p_quien) >= 500 THEN
        RETURN 1;
    END IF;
    INSERT INTO pdf_firma_bloqueos (subject, bloqueado, tenant, nombre)
    VALUES (p_quien, p_bloqueado, coalesce(p_tenant, ''), left(coalesce(p_nombre, ''), 200))
    ON CONFLICT (subject, bloqueado) DO NOTHING;
    UPDATE pdf_peticiones_firma SET silenciada = true
    WHERE destinatario = p_quien AND solicitante = p_bloqueado AND externo AND estado = 'pendiente';
    RETURN 0;
END
$$;

CREATE FUNCTION docs_firma_desbloquear(p_quien text, p_bloqueado text)
RETURNS void
LANGUAGE sql
SECURITY DEFINER
SET search_path = pg_catalog, public
SET app.docs_definer = 'on'
AS $$
    DELETE FROM pdf_firma_bloqueos WHERE subject = p_quien AND bloqueado = p_bloqueado
$$;

CREATE FUNCTION docs_firma_bloqueos(p_quien text)
RETURNS TABLE (bloqueado text, nombre text, creado timestamptz)
LANGUAGE sql
SECURITY DEFINER
SET search_path = pg_catalog, public
SET app.docs_definer = 'on'
STABLE
AS $$
    SELECT bloqueado, nombre, creado FROM pdf_firma_bloqueos
    WHERE subject = p_quien ORDER BY creado DESC LIMIT 500
$$;

-- ¿Ha bloqueado `p_destinatario` a `p_solicitante`? Solo un sí o un no.
CREATE FUNCTION docs_firma_bloqueado(p_destinatario text, p_solicitante text)
RETURNS boolean
LANGUAGE sql
SECURITY DEFINER
SET search_path = pg_catalog, public
SET app.docs_definer = 'on'
STABLE
AS $$
    SELECT EXISTS (SELECT 1 FROM pdf_firma_bloqueos WHERE subject = p_destinatario AND bloqueado = p_solicitante)
$$;

-- La política de cada organización para las peticiones de fuera: abierta (la de
-- siempre, y la de una organización sin fila), cerrada, o solo de las organizaciones
-- de `confianza` (sus slugs, hasta 100).
CREATE TABLE pdf_firma_politica (
    tenant          text        PRIMARY KEY,
    modo            text        NOT NULL DEFAULT 'abierta',
    confianza       text[]      NOT NULL DEFAULT '{}',
    actualizada     timestamptz NOT NULL DEFAULT now(),
    actualizada_por text        NOT NULL DEFAULT '',
    CONSTRAINT pdf_firma_politica_modo_chk CHECK (modo IN ('abierta', 'cerrada', 'confianza')),
    CONSTRAINT pdf_firma_politica_confianza_chk CHECK (cardinality(confianza) <= 100)
);
ALTER TABLE pdf_firma_politica ENABLE ROW LEVEL SECURITY;
ALTER TABLE pdf_firma_politica FORCE ROW LEVEL SECURITY;
CREATE POLICY pdf_firma_politica_tenant_isolation ON pdf_firma_politica
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY pdf_firma_politica_platform_merge ON pdf_firma_politica
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
CREATE POLICY pdf_firma_politica_docs_definer ON pdf_firma_politica FOR SELECT
    USING (current_setting('app.docs_definer', true) = 'on');
GRANT SELECT, INSERT, UPDATE, DELETE ON pdf_firma_politica TO docs_app;

-- ¿Admite alguna de `p_orgs` una petición que viene de `p_origen`? Solo un sí o un
-- no, y sin decir cuál.
CREATE FUNCTION docs_firma_admite(p_orgs text[], p_origen text)
RETURNS boolean
LANGUAGE sql
SECURITY DEFINER
SET search_path = pg_catalog, public
SET app.docs_definer = 'on'
STABLE
AS $$
    SELECT EXISTS (
        SELECT 1 FROM unnest(p_orgs) AS o(slug)
        LEFT JOIN pdf_firma_politica p ON p.tenant = o.slug
        WHERE o.slug <> p_origen
          AND (p.tenant IS NULL OR p.modo = 'abierta'
               OR (p.modo = 'confianza' AND p_origen = ANY (p.confianza)))
    )
$$;

-- Lo que ve quien firma, ahora sin lo silenciado.
CREATE OR REPLACE FUNCTION docs_firma_anfitrion(p_id uuid, p_sub text)
RETURNS text
LANGUAGE sql
SECURITY DEFINER
SET search_path = pg_catalog, public
SET app.docs_definer = 'on'
STABLE
AS $$
    SELECT tenant FROM pdf_peticiones_firma
    WHERE id = p_id AND destinatario = p_sub AND externo AND NOT silenciada
$$;

-- La lista de quien firma: sin lo silenciado y solo lo que admite la organización
-- desde la que mira (`p_tenant`).
CREATE OR REPLACE FUNCTION docs_firmas_de_fuera(p_sub text, p_tenant text)
RETURNS TABLE (id uuid, solicitante text, solicitante_nombre text, destinatario text,
               destinatario_correo text, nombre text, copia text, copia_ruta text,
               mensaje text, estado text, creada timestamptz, resuelta timestamptz)
LANGUAGE sql
SECURITY DEFINER
SET search_path = pg_catalog, public
SET app.docs_definer = 'on'
STABLE
AS $$
    SELECT f.id, f.solicitante, f.solicitante_nombre, f.destinatario, f.destinatario_correo,
           substring(f.ruta from '[^/]*$'), coalesce(f.copia, ''), coalesce(f.copia_ruta, ''),
           f.mensaje, f.estado, f.creada, f.resuelta
    FROM pdf_peticiones_firma f
    WHERE f.destinatario = p_sub AND f.externo AND NOT f.silenciada AND f.tenant <> p_tenant
      AND docs_firma_admite(ARRAY[p_tenant], f.tenant)
    ORDER BY f.creada DESC
    LIMIT 200
$$;

-- El tope de pendientes de fuera de una persona no cuenta lo silenciado: si contara,
-- alguien bloqueado podría seguir llenándolo.
CREATE OR REPLACE FUNCTION docs_firmas_pendientes_para(p_sub text)
RETURNS bigint
LANGUAGE sql
SECURITY DEFINER
SET search_path = pg_catalog, public
SET app.docs_definer = 'on'
STABLE
AS $$
    SELECT count(*) FROM pdf_peticiones_firma
    WHERE destinatario = p_sub AND externo AND estado = 'pendiente' AND NOT silenciada
$$;

-- Fusión de organizaciones (y renombrado, que usa el mismo motor): la política del
-- origen se descarta si el destino tiene la suya —dos filas no caben con PRIMARY KEY
-- (tenant), y la que manda es la de la organización que queda—, y quien confiaba en el
-- origen pasa a confiar en el destino.
CREATE FUNCTION platform_merge_prepare(source text, target text) RETURNS text
LANGUAGE plpgsql
AS $$
DECLARE
    descartada integer;
BEGIN
    -- Solo dentro de una fusión (ver la 0013 de Mail): sin esta guarda, una
    -- organización podría borrarse su propia política llamándola.
    IF current_setting('app.merge', true) IS DISTINCT FROM 'on' THEN
        RAISE EXCEPTION 'platform_merge_prepare: solo puede ejecutarse dentro de una transacción de fusión';
    END IF;
    IF source IS NULL OR target IS NULL OR source = target THEN
        RAISE EXCEPTION 'platform_merge_prepare: origen y destino deben ser distintos (% → %)', source, target;
    END IF;
    DELETE FROM pdf_firma_politica
    WHERE tenant = source AND EXISTS (SELECT 1 FROM pdf_firma_politica WHERE tenant = target);
    GET DIAGNOSTICS descartada = ROW_COUNT;
    UPDATE pdf_firma_politica
    SET confianza = ARRAY(SELECT DISTINCT x FROM unnest(array_replace(confianza, source, target)) AS x
                          WHERE x <> pdf_firma_politica.tenant AND x <> source ORDER BY x)
    WHERE source = ANY (confianza);
    -- La que se queda no confía en sí misma (si confiaba en la que absorbe).
    UPDATE pdf_firma_politica SET confianza = array_remove(confianza, target) WHERE tenant = target;
    IF descartada > 0 THEN
        RETURN 'docs: descartada la política de firmas de fuera de ' || source || ' (queda la de ' || target || ')';
    END IF;
    RETURN NULL;
END
$$;

REVOKE ALL ON FUNCTION docs_firma_bloquear(text, text, text, text) FROM PUBLIC;
REVOKE ALL ON FUNCTION docs_firma_desbloquear(text, text) FROM PUBLIC;
REVOKE ALL ON FUNCTION docs_firma_bloqueos(text) FROM PUBLIC;
REVOKE ALL ON FUNCTION docs_firma_bloqueado(text, text) FROM PUBLIC;
REVOKE ALL ON FUNCTION docs_firma_admite(text[], text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION docs_firma_bloquear(text, text, text, text) TO docs_app;
GRANT EXECUTE ON FUNCTION docs_firma_desbloquear(text, text) TO docs_app;
GRANT EXECUTE ON FUNCTION docs_firma_bloqueos(text) TO docs_app;
GRANT EXECUTE ON FUNCTION docs_firma_bloqueado(text, text) TO docs_app;
GRANT EXECUTE ON FUNCTION docs_firma_admite(text[], text) TO docs_app;
