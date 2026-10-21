-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- «Pedir acceso de edición» desde la franja de solo lectura de un calendario
-- compartido: quien lo tiene de lector se lo pide al dueño. Las columnas son las de
-- platform/pkg/peticiones, iguales en todos los servicios. Una fila por persona y
-- calendario; se borra al conceder o rechazar.
CREATE TABLE calendar_peticiones_edicion (
    calendar_id uuid        NOT NULL REFERENCES calendars (id) ON DELETE CASCADE,
    tenant      text        NOT NULL,
    sujeto      text        NOT NULL,
    nombre      text        NOT NULL DEFAULT '',
    correo      text        NOT NULL DEFAULT '',
    creada      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (calendar_id, sujeto)
);

ALTER TABLE calendar_peticiones_edicion ENABLE ROW LEVEL SECURITY;
ALTER TABLE calendar_peticiones_edicion FORCE ROW LEVEL SECURITY;

CREATE POLICY calendar_peticiones_edicion_tenant_isolation ON calendar_peticiones_edicion
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

-- Fusión de organizaciones (0011 solo cubrió las tablas que ya existían).
CREATE POLICY calendar_peticiones_edicion_platform_merge ON calendar_peticiones_edicion
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
