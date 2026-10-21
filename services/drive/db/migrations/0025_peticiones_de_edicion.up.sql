-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- «Pedir acceso de edición» desde la franja de solo lectura de «Compartido conmigo»:
-- quien tiene un fichero o una carpeta en lectura se lo pide a quien se lo compartió.
-- Las columnas son las de platform/pkg/peticiones, iguales en todos los servicios.
--
-- `recurso` es el id del fichero O de la carpeta compartida, como `shares` (que lleva
-- los dos en columnas aparte); por eso no hay clave ajena: una petición sobre algo que
-- ya no existe no concede nada, porque conceder exige su compartición.
CREATE TABLE drive_peticiones_edicion (
    recurso uuid        NOT NULL,
    tenant  text        NOT NULL,
    sujeto  text        NOT NULL,
    nombre  text        NOT NULL DEFAULT '',
    correo  text        NOT NULL DEFAULT '',
    creada  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (recurso, sujeto)
);

ALTER TABLE drive_peticiones_edicion ENABLE ROW LEVEL SECURITY;
ALTER TABLE drive_peticiones_edicion FORCE ROW LEVEL SECURITY;

CREATE POLICY drive_peticiones_edicion_tenant_isolation ON drive_peticiones_edicion
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

-- Fusión de organizaciones (0015 solo cubrió las tablas que ya existían).
CREATE POLICY drive_peticiones_edicion_platform_merge ON drive_peticiones_edicion
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
