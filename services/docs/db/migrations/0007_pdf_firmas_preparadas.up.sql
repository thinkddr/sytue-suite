-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- Firmas de PDF PREPARADAS y todavía sin completar: el primer paso de la firma con
-- el certificado del sistema (docs/decisions.md#0051).
--
-- POR QUÉ HACE FALTA ESTADO. La firma se parte en dos peticiones: aquí se prepara
-- el documento y el resumen, la app lo firma con la clave del Llavero, del almacén
-- de Windows o de la tarjeta, y vuelve. Lo que liga la vuelta a la ida vive en esta
-- fila, y no en el cliente: la app podría mentir sobre qué se preparó, y con varias
-- réplicas del servicio la ida y la vuelta pueden caer en réplicas distintas.
--
-- QUÉ SE GUARDA y qué no. La huella del PDF preparado (el PDF lo guarda la app y lo
-- devuelve: un documento de 25 MiB no se aparca en la base), los atributos que se
-- firman y la cadena del certificado ELEGIDO —público, pero con el nombre y a veces
-- el DNI de quien firma—. Nada de claves: aquí no llega ninguna. La fila vive diez
-- minutos, se borra al completar y la purga horaria se lleva las caducadas.
CREATE TABLE pdf_firmas_preparadas (
    id             uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant         text        NOT NULL,
    -- `sub` de Sytue ID: solo quien preparó puede completar.
    sujeto         text        NOT NULL,
    creada         timestamptz NOT NULL DEFAULT now(),
    caduca         timestamptz NOT NULL,
    -- Reserva mientras se completa (se pide el sello, se monta el CMS): dos
    -- vueltas a la vez con la misma preparación no pueden producir dos firmas.
    en_curso_hasta timestamptz,
    huella         bytea       NOT NULL,
    hueco_inicio   integer     NOT NULL,
    hueco_fin      integer     NOT NULL,
    atributos      bytea       NOT NULL,
    certificados   bytea       NOT NULL,
    CONSTRAINT pdf_firmas_preparadas_huella_chk CHECK (octet_length(huella) = 32),
    CONSTRAINT pdf_firmas_preparadas_hueco_chk CHECK (hueco_inicio > 0 AND hueco_fin > hueco_inicio)
);

CREATE INDEX pdf_firmas_preparadas_sujeto_idx ON pdf_firmas_preparadas (tenant, sujeto, caduca);
CREATE INDEX pdf_firmas_preparadas_caduca_idx ON pdf_firmas_preparadas (caduca);

-- Aislamiento por organización (deny-by-default), el mismo patrón que 0001 y 0003.
ALTER TABLE pdf_firmas_preparadas ENABLE ROW LEVEL SECURITY;
ALTER TABLE pdf_firmas_preparadas FORCE ROW LEVEL SECURITY;

CREATE POLICY pdf_firmas_preparadas_tenant_isolation ON pdf_firmas_preparadas
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

-- Fusión de organizaciones (0004 solo cubrió las tablas que ya existían).
CREATE POLICY pdf_firmas_preparadas_platform_merge ON pdf_firmas_preparadas
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');

-- La purga de las caducadas es de TODAS las organizaciones a la vez, y solo LEE y
-- BORRA: un DELETE con WHERE también pasa por las políticas de SELECT, así que sin
-- la segunda la purga no vería ninguna fila y borraría cero sin dar error.
CREATE POLICY pdf_firmas_preparadas_purga_ver ON pdf_firmas_preparadas
    FOR SELECT
    USING (current_setting('app.maintenance', true) = 'on');
CREATE POLICY pdf_firmas_preparadas_purga ON pdf_firmas_preparadas
    FOR DELETE
    USING (current_setting('app.maintenance', true) = 'on');

GRANT SELECT, INSERT, UPDATE, DELETE ON pdf_firmas_preparadas TO docs_app;
