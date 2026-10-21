-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Recursos reservables (salas, equipos) con el patrón resource-calendar de
-- Exchange/Google: cada recurso tiene su PROPIO calendario (backing calendar) donde
-- viven sus reservas. Reservar = crear un evento en ese calendario; consultar la
-- disponibilidad = free/busy de ese calendario (reutiliza el motor existente). El
-- recurso AUTO-ACEPTA si está libre en la franja o RECHAZA si está ocupado.
--
-- Seguridad: aislamiento por tenant (RLS deny-by-default como el resto del esquema);
-- el backing calendar pertenece a un dueño sintético 'resource:<id>' que ningún
-- usuario posee, así el recurso jamás aparece en la lista de calendarios de nadie ni
-- contamina el free/busy de un usuario. Solo el gestor (managers/created_by) o un
-- admin del tenant edita el recurso; cualquiera del tenant puede reservar uno activo.
-- Anti doble-reserva: el store toma un FOR UPDATE sobre la fila del recurso antes de
-- comprobar el solape e insertar la reserva, de modo que dos reservas simultáneas de
-- la misma sala/franja se serializan y solo una gana.

CREATE TABLE calendar_resources (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant       text        NOT NULL,
    -- Calendario de reservas del recurso (backing calendar). Al borrar el recurso se
    -- borra en cascada su calendario y, con él, sus reservas.
    calendar_id  uuid        NOT NULL REFERENCES calendars (id) ON DELETE CASCADE,
    kind         text        NOT NULL CHECK (kind IN ('room', 'equipment')),
    name         text        NOT NULL,
    -- aforo (solo salas): 0 = sin especificar.
    capacity     integer     NOT NULL DEFAULT 0 CHECK (capacity >= 0),
    location     text        NOT NULL DEFAULT '',
    -- identidad de calendario del recurso (mailto) para invitarlo como asistente
    -- (ATTENDEE;CUTYPE=ROOM/RESOURCE) en el evento origen. Opcional.
    email        text        NOT NULL DEFAULT '',
    -- auto_accept: aceptación automática por disponibilidad (libre→acepta /
    -- ocupado→rechaza). Si es false, la reserva queda TENTATIVE (pendiente) hasta que
    -- un gestor la confirme o libere.
    auto_accept  boolean     NOT NULL DEFAULT true,
    -- subs de Sytue ID que gestionan el recurso (editar/borrar/aprobar), además de
    -- created_by y de los admin/owner del tenant.
    managers     text[]      NOT NULL DEFAULT ARRAY[]::text[],
    active       boolean     NOT NULL DEFAULT true,
    created_by   text        NOT NULL DEFAULT '',
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    deleted_at   timestamptz,
    CONSTRAINT calendar_resources_name_not_blank CHECK (length(name) > 0)
);

-- «Recursos activos del tenant» (listado + disponibilidad): recorrido indexado.
CREATE INDEX calendar_resources_tenant_idx
    ON calendar_resources (tenant)
    WHERE deleted_at IS NULL;

-- Enlace recurso→calendario (uno a uno) y resolución inversa.
CREATE UNIQUE INDEX calendar_resources_calendar_idx
    ON calendar_resources (tenant, calendar_id)
    WHERE deleted_at IS NULL;

-- «Recursos que gestiono» (sub ∈ managers): búsqueda por pertenencia en el array.
CREATE INDEX calendar_resources_managers_idx
    ON calendar_resources USING GIN (managers);

-- RLS: aislamiento por tenant deny-by-default, como el resto del esquema.
ALTER TABLE calendar_resources ENABLE ROW LEVEL SECURITY;
ALTER TABLE calendar_resources FORCE ROW LEVEL SECURITY;

CREATE POLICY calendar_resources_tenant_isolation ON calendar_resources
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
