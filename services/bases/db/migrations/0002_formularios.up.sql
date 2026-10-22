-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Sytue Bases, formularios públicos: un ENLACE REVOCABLE a una vista de tipo formulario,
-- con el que cualquiera —sin sesión— añade registros a una tabla.
--
-- El token NUNCA se guarda: solo su SHA-256. Lo que se entrega al propietario al crear el
-- enlace es lo único que lo contiene, y se enseña una vez. La búsqueda por token es la
-- única lectura sin tenant del servicio y la hace una función del store que fija
-- `app.bases_publico`; todo lo demás (insertar el registro) va en el tenant del enlace.
--
-- Reglas de TODA tabla con `tenant`: RLS ENABLE + FORCE con aislamiento y fusión, y claves
-- foráneas compuestas (tenant, id) con ON UPDATE CASCADE, como en 0001.

CREATE TABLE base_form_links (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant        text NOT NULL,
    base_id       uuid NOT NULL,
    table_id      uuid NOT NULL,
    view_id       uuid NOT NULL,
    token_hash    text NOT NULL UNIQUE CHECK (token_hash ~ '^[0-9a-f]{64}$'),
    -- Tope de envíos del enlace: un formulario abusado se queda sin cupo y no llena la tabla.
    max_responses integer NOT NULL DEFAULT 500 CHECK (max_responses BETWEEN 1 AND 10000),
    responses     integer NOT NULL DEFAULT 0 CHECK (responses >= 0),
    expires_at    timestamptz,
    revoked_at    timestamptz,
    created_by    text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant, id),
    FOREIGN KEY (tenant, view_id) REFERENCES base_views (tenant, id) ON DELETE CASCADE ON UPDATE CASCADE,
    FOREIGN KEY (tenant, table_id, base_id) REFERENCES base_tables (tenant, id, base_id) ON DELETE CASCADE ON UPDATE CASCADE
);
CREATE INDEX base_form_links_por_vista ON base_form_links (tenant, view_id);

ALTER TABLE base_form_links ENABLE ROW LEVEL SECURITY;
ALTER TABLE base_form_links FORCE ROW LEVEL SECURITY;
CREATE POLICY base_form_links_tenant_isolation ON base_form_links
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY base_form_links_platform_merge ON base_form_links
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
-- Solo lectura, para resolver un token sin saber a qué organización pertenece.
CREATE POLICY base_form_links_publico ON base_form_links FOR SELECT
    USING (current_setting('app.bases_publico', true) = 'on');

-- ALTCHA de un solo uso: SIN tenant ni RLS a propósito. Es la firma anónima de un reto ya
-- resuelto (platform/pkg/altcha) y no pertenece a ninguna organización.
CREATE TABLE base_altcha_used (
    signature  text PRIMARY KEY,
    expires_at timestamptz NOT NULL
);
CREATE INDEX base_altcha_used_caducadas ON base_altcha_used (expires_at);
