-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- «Pedir acceso de edición» desde la franja de solo lectura de un módulo del ERP:
-- quien tiene el nivel `lectura` se lo pide a la administración de la organización,
-- que lo sube a `usuario` con un clic. Las columnas son las de platform/pkg/peticiones,
-- iguales en todos los servicios; `recurso` es la clave del módulo.
CREATE TABLE erp_peticiones_edicion (
    recurso text        NOT NULL,
    tenant  text        NOT NULL,
    sujeto  text        NOT NULL,
    nombre  text        NOT NULL DEFAULT '',
    correo  text        NOT NULL DEFAULT '',
    creada  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, recurso, sujeto)
);

ALTER TABLE erp_peticiones_edicion ENABLE ROW LEVEL SECURITY;
ALTER TABLE erp_peticiones_edicion FORCE ROW LEVEL SECURITY;

CREATE POLICY erp_peticiones_edicion_tenant_isolation ON erp_peticiones_edicion
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY erp_peticiones_edicion_platform_merge ON erp_peticiones_edicion
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
