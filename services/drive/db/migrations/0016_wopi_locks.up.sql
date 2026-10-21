-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- BLOQUEO WOPI: un editor externo (Office Online, Collabora, OnlyOffice) reclama un
-- fichero de Sytue Drive mientras lo tiene abierto.
--
-- No es un capricho del protocolo: WOPI no tiene fusión de cambios. Un PutFile reemplaza
-- el contenido entero, así que si dos editores abren el mismo fichero, el segundo en
-- guardar borra el trabajo del primero sin avisar a nadie. El bloqueo es lo que convierte
-- esa pérdida silenciosa en un 409 que el editor sabe explicar.
--
-- El lock_id lo elige el EDITOR y es opaco para nosotros: la especificación solo exige
-- devolverlo tal cual en X-WOPI-Lock y comparar por igualdad exacta. No se interpreta.
--
-- Una fila POR FICHERO (clave primaria en file_id): un fichero o está bloqueado o no.
-- Los locks caducan solos —expires_at— porque un editor puede morirse sin soltar el
-- suyo, y sin caducidad ese fichero quedaría inmovilizado para siempre.
CREATE TABLE wopi_locks (
    tenant     text        NOT NULL,
    file_id    uuid        NOT NULL,
    -- Identificador opaco del editor. Se compara por igualdad EXACTA (WOPI §Lock).
    lock_id    text        NOT NULL,
    -- Quién lo tomó: para poder explicar «lo tiene abierto Ana» en vez de un 409 mudo.
    holder_sub text        NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (file_id),
    -- Si el fichero se borra, su bloqueo se va con él: un lock huérfano bloquearía un
    -- id que ya no existe y no habría forma de soltarlo.
    CONSTRAINT wopi_locks_file_fk FOREIGN KEY (file_id) REFERENCES files (id) ON DELETE CASCADE
);

-- Barrido de los caducados sin recorrer la tabla entera.
CREATE INDEX wopi_locks_expiry_idx ON wopi_locks (expires_at);

-- Aislamiento por organización, igual que el resto del servicio. FORCE incluido: el
-- dueño de la tabla tampoco se salta la política (docs/security.md).
ALTER TABLE wopi_locks ENABLE ROW LEVEL SECURITY;
ALTER TABLE wopi_locks FORCE ROW LEVEL SECURITY;

CREATE POLICY wopi_locks_isolation ON wopi_locks
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

-- Fusión de organizaciones: misma política PERMISSIVE acotada que el resto de tablas con
-- columna `tenant` (ver 0015). Sin ella, fusionar dejaría atrás los bloqueos de la
-- organización de origen y el motor lo daría por completado.
CREATE POLICY wopi_locks_merge ON wopi_locks
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
