-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- BIBLIOTECA DE PLANTILLAS POR ORGANIZACIÓN (ROADMAP-OFFICE plantillas-1).
--
-- Un administrador marca un documento, una hoja o una presentación como plantilla de
-- su organización y aparece en la galería de «Crear nuevo» de todos sus miembros.
-- Es solo una MARCA sobre un documento que ya existe: no copia nada y quitarla no
-- borra el documento. Dos tablas nuevas, ninguna toca a las existentes:
--
--   · doc_org_templates: la marca (una fila por documento marcado) con el nombre y la
--     descripción con que se ofrece en la galería.
--   · doc_org_template_log: el registro de lo que se hace con cada plantilla (marcar,
--     publicar versión, quitar, usar). Es también donde viven las VERSIONES: publicar
--     una versión es sacar una instantánea del documento (doc_versions) y apuntarla
--     aquí con su número. El documento se sigue editando a su ritmo; a los miembros
--     solo les llega lo PUBLICADO, y quien crea desde la plantilla elige la versión
--     (la última publicada por defecto). Sin clave foránea a documents a propósito:
--     el registro es auditoría y sobrevive a que el documento se purgue.
CREATE TABLE doc_org_templates (
    document_id uuid        PRIMARY KEY REFERENCES documents (id) ON DELETE CASCADE,
    tenant      text        NOT NULL,
    kind        doc_kind    NOT NULL,
    name        text        NOT NULL,
    description text        NOT NULL DEFAULT '',
    marked_by   text        NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX doc_org_templates_tenant_idx ON doc_org_templates (tenant, kind);

CREATE TABLE doc_org_template_log (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant      text        NOT NULL,
    document_id uuid        NOT NULL,
    action      text        NOT NULL CHECK (action IN ('marked', 'published', 'renamed', 'unmarked', 'used')),
    actor_sub   text        NOT NULL,
    -- Solo en 'marked' y 'published': la instantánea y su número de versión.
    version_id  uuid        REFERENCES doc_versions (id) ON DELETE SET NULL,
    version_no  integer,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX doc_org_template_log_doc_idx ON doc_org_template_log (document_id, created_at DESC);

-- Aislamiento por organización (deny-by-default), el mismo patrón que 0001 y 0016.
ALTER TABLE doc_org_templates ENABLE ROW LEVEL SECURITY;
ALTER TABLE doc_org_templates FORCE ROW LEVEL SECURITY;
CREATE POLICY doc_org_templates_tenant_isolation ON doc_org_templates
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY doc_org_templates_platform_merge ON doc_org_templates
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');

ALTER TABLE doc_org_template_log ENABLE ROW LEVEL SECURITY;
ALTER TABLE doc_org_template_log FORCE ROW LEVEL SECURITY;
CREATE POLICY doc_org_template_log_tenant_isolation ON doc_org_template_log
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY doc_org_template_log_platform_merge ON doc_org_template_log
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');

GRANT SELECT, INSERT, UPDATE, DELETE ON doc_org_templates, doc_org_template_log TO docs_app;
