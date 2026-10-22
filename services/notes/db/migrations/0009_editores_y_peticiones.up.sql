-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- Editar el cuaderno de otra persona y «Pedir acceso de edición» desde la franja de
-- solo lectura.
--
-- Hasta aquí un cuaderno ajeno se escribía solo si era de la organización entera: uno
-- PÚBLICO se lee y no se escribe. `notebook_editores` es el permiso de UNA persona para
-- escribir las notas de un cuaderno que lee; lo concede su dueño. El cuaderno en sí
-- —título, visibilidad, subcuadernos— sigue siendo solo del dueño.
CREATE TABLE notebook_editores (
    notebook_id   uuid        NOT NULL REFERENCES notebooks (id) ON DELETE CASCADE,
    tenant        text        NOT NULL,
    sujeto        text        NOT NULL,
    concedido_por text        NOT NULL,
    creado        timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (notebook_id, sujeto)
);

-- Las columnas son las de platform/pkg/peticiones, iguales en todos los servicios.
CREATE TABLE notebook_peticiones_edicion (
    notebook_id uuid        NOT NULL REFERENCES notebooks (id) ON DELETE CASCADE,
    tenant      text        NOT NULL,
    sujeto      text        NOT NULL,
    nombre      text        NOT NULL DEFAULT '',
    correo      text        NOT NULL DEFAULT '',
    creada      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (notebook_id, sujeto)
);

ALTER TABLE notebook_editores ENABLE ROW LEVEL SECURITY;
ALTER TABLE notebook_editores FORCE ROW LEVEL SECURITY;
CREATE POLICY notebook_editores_tenant_isolation ON notebook_editores
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY notebook_editores_platform_merge ON notebook_editores
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');

ALTER TABLE notebook_peticiones_edicion ENABLE ROW LEVEL SECURITY;
ALTER TABLE notebook_peticiones_edicion FORCE ROW LEVEL SECURITY;
CREATE POLICY notebook_peticiones_edicion_tenant_isolation ON notebook_peticiones_edicion
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY notebook_peticiones_edicion_platform_merge ON notebook_peticiones_edicion
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
