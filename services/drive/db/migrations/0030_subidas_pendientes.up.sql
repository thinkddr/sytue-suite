-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- LAS SUBIDAS DIRECTAS PENDIENTES OCUPAN SITIO (FILE-08).
--
-- Una URL prefirmada de subida no se puede revocar ni ata el tamaño: quien la
-- pedía sin llamar después a finalizar dejaba en el almacén lo que quisiera, sin
-- fila y sin cuota, para siempre. Ahora cada URL deja aquí su reserva: lo
-- pendiente cuenta contra la cuota al pedir otra, hay un tope de pendientes por
-- organización, y el recolector borra el objeto de la clave cuando la URL ya no
-- vale (si se finalizó, el fichero vive en otra clave y este objeto sobra).
CREATE TABLE subidas_pendientes (
    tenant      text        NOT NULL,
    storage_key text        NOT NULL,
    size_bytes  bigint      NOT NULL,
    caduca_en   timestamptz NOT NULL,
    finalizada  boolean     NOT NULL DEFAULT false,
    -- Lo que va al bucket propio de la organización no gasta cuota de Sytue.
    con_cuota   boolean     NOT NULL,
    PRIMARY KEY (tenant, storage_key)
);

CREATE INDEX subidas_pendientes_caduca_idx ON subidas_pendientes (caduca_en);

ALTER TABLE subidas_pendientes ENABLE ROW LEVEL SECURITY;
ALTER TABLE subidas_pendientes FORCE ROW LEVEL SECURITY;

CREATE POLICY subidas_pendientes_tenant_isolation ON subidas_pendientes
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

-- El recolector las recorre de todas las organizaciones (como la auto-purga, 0009).
CREATE POLICY subidas_pendientes_maintenance ON subidas_pendientes
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');

CREATE POLICY subidas_pendientes_platform_merge ON subidas_pendientes
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
