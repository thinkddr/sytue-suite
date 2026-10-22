-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- VISTAS DE FILTRO PERSONALES DE CALCUS, EN EL SERVIDOR (ROADMAP-OFFICE datos-1).
--
-- Hasta ahora vivían en el localStorage de cada navegador (por usuario y documento) y no
-- viajaban entre dispositivos. Una vista es una foto con nombre de los criterios del
-- autofiltro y es DE UNA PERSONA: la lee y la escribe solo ella, ni siquiera el
-- propietario del documento ve las de otra. Por eso no se guarda en el modelo compartido
-- (doc:sheet) sino aquí, una fila por (documento, persona) con el estado entero que ya
-- sabe leer el cliente (`{views, active}` por hoja). El servidor acota tamaño y forma;
-- los criterios los valida el cliente al leerlos, igual que antes.
CREATE TABLE doc_vistas_filtro (
    document_id uuid        NOT NULL REFERENCES documents (id) ON DELETE CASCADE,
    tenant      text        NOT NULL,
    sub         text        NOT NULL,
    estado      jsonb       NOT NULL,
    actualizada timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (document_id, sub)
);

-- Aislamiento por organización (deny-by-default), el mismo patrón que 0001, 0008 y 0016.
ALTER TABLE doc_vistas_filtro ENABLE ROW LEVEL SECURITY;
ALTER TABLE doc_vistas_filtro FORCE ROW LEVEL SECURITY;
CREATE POLICY doc_vistas_filtro_tenant_isolation ON doc_vistas_filtro
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY doc_vistas_filtro_platform_merge ON doc_vistas_filtro
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');

GRANT SELECT, INSERT, UPDATE, DELETE ON doc_vistas_filtro TO docs_app;
