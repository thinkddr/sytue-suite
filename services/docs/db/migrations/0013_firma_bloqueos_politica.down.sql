-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- Sin bloqueos ni política, las silenciadas llegarían de golpe: se cancelan antes. Quien
-- las pidió las verá «canceladas» sin haberlo hecho: es lo único que cuenta la bajada, y
-- es menos que dejarlas llegar. Dentro de un DO para que el `app.merge` valga seguro
-- para el UPDATE, lo ejecute el runner en una transacción o no.
DO $$
BEGIN
    PERFORM set_config('app.merge', 'on', true);
    UPDATE pdf_peticiones_firma SET estado = 'cancelada', resuelta = now(), enlace = NULL
    WHERE silenciada AND estado = 'pendiente';
    PERFORM set_config('app.merge', 'off', true);
END
$$;
DROP FUNCTION IF EXISTS platform_merge_prepare(text, text);
CREATE OR REPLACE FUNCTION docs_firmas_pendientes_para(p_sub text)
RETURNS bigint LANGUAGE sql SECURITY DEFINER SET search_path = pg_catalog, public STABLE AS $$
    SELECT count(*) FROM pdf_peticiones_firma
    WHERE destinatario = p_sub AND externo AND estado = 'pendiente'
$$;
ALTER FUNCTION docs_firmas_pendientes_para(text) RESET app.docs_definer;
CREATE OR REPLACE FUNCTION docs_firmas_de_fuera(p_sub text, p_tenant text)
RETURNS TABLE (id uuid, solicitante text, solicitante_nombre text, destinatario text,
               destinatario_correo text, nombre text, copia text, copia_ruta text,
               mensaje text, estado text, creada timestamptz, resuelta timestamptz)
LANGUAGE sql SECURITY DEFINER SET search_path = pg_catalog, public STABLE AS $$
    SELECT id, solicitante, solicitante_nombre, destinatario, destinatario_correo,
           substring(ruta from '[^/]*$'), coalesce(copia, ''), coalesce(copia_ruta, ''),
           mensaje, estado, creada, resuelta
    FROM pdf_peticiones_firma
    WHERE destinatario = p_sub AND externo AND tenant <> p_tenant
    ORDER BY creada DESC
    LIMIT 200
$$;
ALTER FUNCTION docs_firmas_de_fuera(text, text) RESET app.docs_definer;
CREATE OR REPLACE FUNCTION docs_firma_anfitrion(p_id uuid, p_sub text)
RETURNS text LANGUAGE sql SECURITY DEFINER SET search_path = pg_catalog, public STABLE AS $$
    SELECT tenant FROM pdf_peticiones_firma
    WHERE id = p_id AND destinatario = p_sub AND externo
$$;
ALTER FUNCTION docs_firma_anfitrion(uuid, text) RESET app.docs_definer;
DROP FUNCTION IF EXISTS docs_firma_admite(text[], text);
DROP TABLE IF EXISTS pdf_firma_politica;
DROP FUNCTION IF EXISTS docs_firma_bloqueado(text, text);
DROP FUNCTION IF EXISTS docs_firma_bloqueos(text);
DROP FUNCTION IF EXISTS docs_firma_desbloquear(text, text);
DROP FUNCTION IF EXISTS docs_firma_bloquear(text, text, text, text);
DROP TABLE IF EXISTS pdf_firma_bloqueos;
ALTER FUNCTION docs_de_fuera(text, text, text, text, boolean, text, timestamptz, uuid, int) RESET app.docs_definer;
ALTER FUNCTION docs_anfitrion(uuid, text) RESET app.docs_definer;
ALTER FUNCTION docs_canjear_enlace(text) RESET app.docs_definer;
ALTER FUNCTION docs_resolve_share(text) RESET app.docs_definer;
DROP POLICY IF EXISTS pdf_peticiones_firma_docs_definer_silenciar ON pdf_peticiones_firma;
DROP POLICY IF EXISTS pdf_peticiones_firma_docs_definer_ver ON pdf_peticiones_firma;
DROP POLICY IF EXISTS doc_permissions_docs_definer ON doc_permissions;
DROP POLICY IF EXISTS doc_share_links_docs_definer ON doc_share_links;
DROP POLICY IF EXISTS documents_docs_definer ON documents;
ALTER TABLE pdf_peticiones_firma
    DROP CONSTRAINT IF EXISTS pdf_peticiones_firma_silenciada_chk,
    DROP COLUMN IF EXISTS silenciada;
