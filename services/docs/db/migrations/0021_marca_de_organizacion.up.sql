-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- MARCA DE LA ORGANIZACIÓN, VERSIONADA (ROADMAP-OFFICE plantillas-1).
--
-- Los colores y las tipografías de la organización, que Docus, Calcus y Diapos ofrecen
-- como un tema más al crear un documento y al diseñarlo. Cada cambio es una versión
-- NUEVA (el número sube de uno en uno por organización) y las anteriores se conservan:
-- un documento puede volver a aplicar la que usó. La vigente es la de número más alto.
-- Tabla nueva, ninguna toca a las existentes.
CREATE TABLE doc_org_brand (
    tenant       text        NOT NULL,
    version      integer     NOT NULL CHECK (version >= 1),
    name         text        NOT NULL,
    bg           text        NOT NULL,
    ink          text        NOT NULL,
    accent       text        NOT NULL,
    heading_font text        NOT NULL DEFAULT '',
    body_font    text        NOT NULL DEFAULT '',
    created_by   text        NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, version)
);

-- Aislamiento por organización (deny-by-default), el mismo patrón que 0001 y 0016.
ALTER TABLE doc_org_brand ENABLE ROW LEVEL SECURITY;
ALTER TABLE doc_org_brand FORCE ROW LEVEL SECURITY;
CREATE POLICY doc_org_brand_tenant_isolation ON doc_org_brand
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY doc_org_brand_platform_merge ON doc_org_brand
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');

GRANT SELECT, INSERT, UPDATE, DELETE ON doc_org_brand TO docs_app;
