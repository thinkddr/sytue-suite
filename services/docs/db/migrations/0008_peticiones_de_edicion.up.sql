-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- «Pedir acceso de edición» desde la franja de solo lectura (Docus, Calcus, Diapos).
--
-- Una fila por persona y documento: pedirlo dos veces no manda dos avisos al dueño
-- ni deja dos filas. Se borra al conceder o al rechazar. Guarda el nombre y el
-- correo de quien pide tal como venían en SU token, porque el dueño tiene que saber
-- a quién concede y el servicio no puede preguntarle a Sytue ID sin la sesión de
-- nadie; el permiso, en cambio, se concede contra el `sub`, que no se puede falsear.
CREATE TABLE doc_peticiones_edicion (
    document_id uuid        NOT NULL REFERENCES documents (id) ON DELETE CASCADE,
    tenant      text        NOT NULL,
    sujeto      text        NOT NULL,
    nombre      text        NOT NULL DEFAULT '',
    correo      text        NOT NULL DEFAULT '',
    creada      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (document_id, sujeto)
);

-- Aislamiento por organización (deny-by-default), el mismo patrón que 0001 y 0007.
ALTER TABLE doc_peticiones_edicion ENABLE ROW LEVEL SECURITY;
ALTER TABLE doc_peticiones_edicion FORCE ROW LEVEL SECURITY;

CREATE POLICY doc_peticiones_edicion_tenant_isolation ON doc_peticiones_edicion
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

-- Fusión de organizaciones (0004 solo cubrió las tablas que ya existían).
CREATE POLICY doc_peticiones_edicion_platform_merge ON doc_peticiones_edicion
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');

GRANT SELECT, INSERT, UPDATE, DELETE ON doc_peticiones_edicion TO docs_app;
