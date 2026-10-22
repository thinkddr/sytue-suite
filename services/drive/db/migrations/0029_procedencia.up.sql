-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- Procedencia de un fichero que Drive no recibió de una persona sino que LEYÓ de otro
-- producto: hoy, una vista de Sytue Bases exportada como CSV (ADR 0068, bases-2). Dice de
-- qué base, tabla y vista salió, quién concedió la conexión y cuándo, y qué versión del
-- fichero es. Una fila por versión exportada: reexportar no borra de dónde vino la anterior.
--
-- La clave ajena es COMPUESTA (tenant, id): una comprobación de clave la hace el
-- propietario y se salta la RLS, así que sin el tenant en la clave la procedencia de una
-- organización podría colgar del fichero de otra. `files` no tenía esa unicidad (su clave
-- primaria es `id`, así que el índice no puede fallar) y se añade aquí. Diferida, con
-- ON UPDATE CASCADE: la fusión de organizaciones reescribe `tenant` tabla a tabla.
ALTER TABLE files ADD CONSTRAINT files_tenant_id_key UNIQUE (tenant, id);

CREATE TABLE drive_file_provenance (
    tenant     text        NOT NULL,
    file_id    uuid        NOT NULL,
    version    integer     NOT NULL CHECK (version >= 1),
    source     text        NOT NULL CHECK (source IN ('sytue-bases')),
    -- Lo que contó el origen, tal cual (sin credenciales: el token no llega aquí).
    detail     jsonb       NOT NULL,
    created_by text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (file_id, version),
    FOREIGN KEY (tenant, file_id) REFERENCES files (tenant, id)
        ON DELETE CASCADE ON UPDATE CASCADE DEFERRABLE INITIALLY DEFERRED
);

ALTER TABLE drive_file_provenance ENABLE ROW LEVEL SECURITY;
ALTER TABLE drive_file_provenance FORCE ROW LEVEL SECURITY;

CREATE POLICY drive_file_provenance_tenant_isolation ON drive_file_provenance
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY drive_file_provenance_platform_merge ON drive_file_provenance
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
