-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Esquema base de Sytue Calendar: calendarios (colecciones CalDAV), objetos de
-- calendario (recursos VEVENT con su iCalendar canonico), eventos normalizados,
-- recurrencias RFC 5545 (RRULE/RDATE/EXDATE) con overrides, asistentes y
-- invitaciones (scheduling iTIP). Estado en PostgreSQL (unica fuente de verdad);
-- sin Redis/Valkey ni motores AGPL.
--
-- Clean-room: modelo propio sobre los estandares abiertos iCalendar (RFC 5545),
-- CalDAV (RFC 4791), scheduling CalDAV (RFC 6638) e iTIP (RFC 5546). No deriva
-- de codigo de terceros AGPL.
--
-- Multi-tenant: cada fila lleva `tenant` (slug). RLS deny-by-default aisla por
-- la variable de sesion `app.tenant` (la fija pg.SetTenant en cada transaccion).

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ---------------------------------------------------------------------------
-- Calendarios: una coleccion CalDAV (RFC 4791) por usuario/tenant. ctag/sync
-- soportan la sincronizacion incremental (RFC 6578). owner_sub es el sujeto
-- (claim "sub") de Sytue ID propietario de la coleccion.
-- ---------------------------------------------------------------------------
CREATE TABLE calendars (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant          text        NOT NULL,
    owner_sub       text        NOT NULL,
    -- name es el segmento de ruta CalDAV (URL-safe, estable). Unico por dueno.
    name            text        NOT NULL,
    display_name    text        NOT NULL DEFAULT '',
    description     text        NOT NULL DEFAULT '',
    -- color de presentacion (#RRGGBB[AA]) — propiedad CalDAV calendar-color.
    color           text        NOT NULL DEFAULT '#3B82F6',
    timezone        text        NOT NULL DEFAULT 'UTC',
    -- components soportados por la coleccion (RFC 4791 supported-calendar-
    -- component-set). Por defecto solo VEVENT.
    components      text[]      NOT NULL DEFAULT ARRAY['VEVENT']::text[],
    is_default      boolean     NOT NULL DEFAULT false,
    -- ctag cambia con cualquier modificacion del contenido (getctag CalDAV).
    ctag            uuid        NOT NULL DEFAULT gen_random_uuid(),
    -- sync_token monotono para REPORT sync-collection (RFC 6578).
    sync_token      bigint      NOT NULL DEFAULT 1,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    deleted_at      timestamptz,
    CONSTRAINT calendars_name_not_blank CHECK (length(name) > 0),
    CONSTRAINT calendars_color_format CHECK (color ~ '^#[0-9A-Fa-f]{6}([0-9A-Fa-f]{2})?$')
);

-- Nombre de coleccion unico por dueno (case-insensitive) entre las vivas.
CREATE UNIQUE INDEX calendars_unique_name
    ON calendars (tenant, owner_sub, lower(name))
    WHERE deleted_at IS NULL;

-- Un unico calendario por defecto por dueno.
CREATE UNIQUE INDEX calendars_one_default
    ON calendars (tenant, owner_sub)
    WHERE is_default AND deleted_at IS NULL;

CREATE INDEX calendars_owner_idx ON calendars (tenant, owner_sub) WHERE deleted_at IS NULL;

-- ---------------------------------------------------------------------------
-- Objetos de calendario: el RECURSO CalDAV (RFC 4791). Un objeto agrupa el
-- VEVENT "maestro" y, opcionalmente, sus overrides de recurrencia, compartiendo
-- UID. Guardamos el iCalendar canonico (data) para servirlo verbatim por CalDAV
-- y el etag para condicionales (If-Match / If-None-Match).
-- ---------------------------------------------------------------------------
CREATE TABLE calendar_objects (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant          text        NOT NULL,
    calendar_id     uuid        NOT NULL REFERENCES calendars (id) ON DELETE CASCADE,
    -- uid es el UID de iCalendar (RFC 5545 3.8.4.7), unico dentro del calendario.
    uid             text        NOT NULL,
    -- resource_name es el nombre de fichero del recurso CalDAV (suele ser
    -- "<uid>.ics"); identifica el recurso en la URL de la coleccion.
    resource_name   text        NOT NULL,
    -- component es el tipo principal del objeto (VEVENT por defecto).
    component       text        NOT NULL DEFAULT 'VEVENT' CHECK (component IN ('VEVENT', 'VTODO', 'VJOURNAL')),
    -- data es el iCalendar serializado canonico (VCALENDAR completo).
    data            text        NOT NULL,
    -- etag valida operaciones condicionales; cambia en cada escritura.
    etag            uuid        NOT NULL DEFAULT gen_random_uuid(),
    -- size en bytes del recurso (getcontentlength CalDAV).
    size_bytes      integer     NOT NULL DEFAULT 0,
    -- sequence_no es el SEQUENCE de iTIP (RFC 5546) del objeto, para scheduling.
    sequence_no     integer     NOT NULL DEFAULT 0,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    deleted_at      timestamptz,
    CONSTRAINT calobj_uid_not_blank CHECK (length(uid) > 0),
    CONSTRAINT calobj_resource_not_blank CHECK (length(resource_name) > 0)
);

-- UID unico por calendario (entre objetos vivos).
CREATE UNIQUE INDEX calobj_unique_uid
    ON calendar_objects (tenant, calendar_id, uid)
    WHERE deleted_at IS NULL;

-- resource_name unico por calendario (la URL del recurso CalDAV).
CREATE UNIQUE INDEX calobj_unique_resource
    ON calendar_objects (tenant, calendar_id, resource_name)
    WHERE deleted_at IS NULL;

CREATE INDEX calobj_calendar_idx ON calendar_objects (tenant, calendar_id) WHERE deleted_at IS NULL;

-- ---------------------------------------------------------------------------
-- Eventos: proyeccion normalizada del VEVENT maestro de un objeto, para la API
-- REST, consultas por rango (calendar-query / free-busy) y recurrencias. El
-- objeto CalDAV (calendar_objects.data) sigue siendo la fuente serializada; esta
-- tabla es el indice consultable.
-- ---------------------------------------------------------------------------
CREATE TABLE events (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant          text        NOT NULL,
    calendar_id     uuid        NOT NULL REFERENCES calendars (id) ON DELETE CASCADE,
    object_id       uuid        NOT NULL REFERENCES calendar_objects (id) ON DELETE CASCADE,
    uid             text        NOT NULL,
    summary         text        NOT NULL DEFAULT '',
    description     text        NOT NULL DEFAULT '',
    location        text        NOT NULL DEFAULT '',
    -- organizador (mailto/sub) — RFC 5545 ORGANIZER.
    organizer       text        NOT NULL DEFAULT '',
    -- dtstart/dtend en UTC. Para eventos de dia completo (all_day) se interpreta
    -- la parte de fecha; tzid conserva la zona horaria original declarada.
    dtstart         timestamptz NOT NULL,
    dtend           timestamptz NOT NULL,
    tzid            text        NOT NULL DEFAULT 'UTC',
    all_day         boolean     NOT NULL DEFAULT false,
    -- status: CONFIRMED | TENTATIVE | CANCELLED (RFC 5545 3.8.1.11).
    status          text        NOT NULL DEFAULT 'CONFIRMED'
                        CHECK (status IN ('CONFIRMED', 'TENTATIVE', 'CANCELLED')),
    -- classification: PUBLIC | PRIVATE | CONFIDENTIAL (RFC 5545 3.8.1.3).
    classification  text        NOT NULL DEFAULT 'PUBLIC'
                        CHECK (classification IN ('PUBLIC', 'PRIVATE', 'CONFIDENTIAL')),
    -- transp: OPAQUE (cuenta para free/busy) | TRANSPARENT (RFC 5545 3.8.2.7).
    transp          text        NOT NULL DEFAULT 'OPAQUE'
                        CHECK (transp IN ('OPAQUE', 'TRANSPARENT')),
    -- recurrencia (RFC 5545): regla RRULE serializada + RDATE/EXDATE. NULL/vacio
    -- => evento puntual. recurrence_end es el ultimo instante alcanzable por la
    -- regla (UNTIL/derivado de COUNT) o NULL si es infinita; acota las consultas.
    rrule           text        NOT NULL DEFAULT '',
    rdate           timestamptz[] NOT NULL DEFAULT ARRAY[]::timestamptz[],
    exdate          timestamptz[] NOT NULL DEFAULT ARRAY[]::timestamptz[],
    recurrence_end  timestamptz,
    -- recurrence_id marca este evento como un OVERRIDE de una instancia concreta
    -- de la serie (RFC 5545 3.8.4.4). NULL en el evento maestro.
    recurrence_id   timestamptz,
    sequence_no     integer     NOT NULL DEFAULT 0,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    deleted_at      timestamptz,
    CONSTRAINT events_time_order CHECK (dtend >= dtstart)
);

-- Maestro: un unico evento sin recurrence_id por objeto. Los overrides comparten
-- object_id pero llevan recurrence_id distinto.
CREATE UNIQUE INDEX events_master_per_object
    ON events (tenant, object_id)
    WHERE recurrence_id IS NULL AND deleted_at IS NULL;

CREATE UNIQUE INDEX events_override_per_object
    ON events (tenant, object_id, recurrence_id)
    WHERE recurrence_id IS NOT NULL AND deleted_at IS NULL;

-- Indice de rango para calendar-query / free-busy: eventos vivos por calendario
-- y ventana temporal. recurrence_end acota las series.
CREATE INDEX events_range_idx
    ON events (tenant, calendar_id, dtstart, dtend)
    WHERE deleted_at IS NULL;

CREATE INDEX events_uid_idx ON events (tenant, uid) WHERE deleted_at IS NULL;

-- ---------------------------------------------------------------------------
-- Asistentes: ATTENDEE de un evento (RFC 5545 3.8.4.1). partstat lleva la
-- respuesta del invitado (NEEDS-ACTION/ACCEPTED/DECLINED/TENTATIVE).
-- ---------------------------------------------------------------------------
CREATE TABLE attendees (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant          text        NOT NULL,
    event_id        uuid        NOT NULL REFERENCES events (id) ON DELETE CASCADE,
    -- email/calendar address del asistente ("mailto:..." normalizado a email).
    email           text        NOT NULL,
    common_name     text        NOT NULL DEFAULT '',
    -- role: CHAIR | REQ-PARTICIPANT | OPT-PARTICIPANT | NON-PARTICIPANT.
    role            text        NOT NULL DEFAULT 'REQ-PARTICIPANT'
                        CHECK (role IN ('CHAIR', 'REQ-PARTICIPANT', 'OPT-PARTICIPANT', 'NON-PARTICIPANT')),
    -- partstat: estado de participacion (RFC 5545 3.2.12).
    partstat        text        NOT NULL DEFAULT 'NEEDS-ACTION'
                        CHECK (partstat IN ('NEEDS-ACTION', 'ACCEPTED', 'DECLINED', 'TENTATIVE', 'DELEGATED')),
    -- cutype: INDIVIDUAL | GROUP | RESOURCE | ROOM | UNKNOWN.
    cutype          text        NOT NULL DEFAULT 'INDIVIDUAL'
                        CHECK (cutype IN ('INDIVIDUAL', 'GROUP', 'RESOURCE', 'ROOM', 'UNKNOWN')),
    rsvp            boolean     NOT NULL DEFAULT true,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT attendees_email_not_blank CHECK (length(email) > 0)
);

CREATE UNIQUE INDEX attendees_unique
    ON attendees (tenant, event_id, lower(email));
CREATE INDEX attendees_event_idx ON attendees (tenant, event_id);
CREATE INDEX attendees_email_idx ON attendees (tenant, lower(email));

-- ---------------------------------------------------------------------------
-- Invitaciones / scheduling (iTIP, RFC 5546 + CalDAV scheduling, RFC 6638).
-- Cola transaccional de mensajes de planificacion entregados a la "scheduling
-- inbox" del destinatario, dentro del tenant.
-- ---------------------------------------------------------------------------
CREATE TABLE invitations (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant          text        NOT NULL,
    event_id        uuid        REFERENCES events (id) ON DELETE SET NULL,
    uid             text        NOT NULL,
    -- method iTIP: REQUEST | REPLY | CANCEL | ADD | REFRESH | COUNTER | DECLINECOUNTER.
    method          text        NOT NULL
                        CHECK (method IN ('REQUEST', 'REPLY', 'CANCEL', 'ADD', 'REFRESH', 'COUNTER', 'DECLINECOUNTER')),
    organizer       text        NOT NULL DEFAULT '',
    recipient       text        NOT NULL,
    -- sub de Sytue ID del destinatario (si es un usuario interno del tenant).
    recipient_sub   text        NOT NULL DEFAULT '',
    sequence_no     integer     NOT NULL DEFAULT 0,
    -- payload iCalendar (VCALENDAR con METHOD) del mensaje de scheduling.
    data            text        NOT NULL DEFAULT '',
    -- status del procesamiento de la invitacion por el destinatario.
    status          text        NOT NULL DEFAULT 'pending'
                        CHECK (status IN ('pending', 'accepted', 'declined', 'tentative', 'delivered', 'cancelled')),
    created_at      timestamptz NOT NULL DEFAULT now(),
    responded_at    timestamptz,
    CONSTRAINT invitations_recipient_not_blank CHECK (length(recipient) > 0)
);

CREATE INDEX invitations_recipient_idx
    ON invitations (tenant, lower(recipient), status, created_at);
CREATE INDEX invitations_event_idx ON invitations (tenant, event_id);
CREATE INDEX invitations_uid_idx ON invitations (tenant, uid);

-- ---------------------------------------------------------------------------
-- Outbox de eventos (sobre PostgreSQL; sin Redis/Valkey). DDL compartido de
-- platform/pkg/events, replicado aqui porque cada servicio gestiona su esquema.
-- ---------------------------------------------------------------------------
CREATE TABLE events_outbox (
    id           uuid        PRIMARY KEY,
    channel      text        NOT NULL,
    type         text        NOT NULL,
    tenant       text        NOT NULL DEFAULT '',
    payload      jsonb       NOT NULL DEFAULT '{}'::jsonb,
    created_at   timestamptz NOT NULL DEFAULT now(),
    processed_at timestamptz,
    attempts     int         NOT NULL DEFAULT 0,
    last_error   text
);

CREATE INDEX events_outbox_unprocessed_idx
    ON events_outbox (created_at)
    WHERE processed_at IS NULL;

-- ---------------------------------------------------------------------------
-- Row-Level Security: aislamiento por tenant deny-by-default (CONVENTIONS §10).
-- Las politicas comparan `tenant` con current_setting('app.tenant', true), que
-- fija pg.SetTenant LOCAL a cada transaccion de negocio.
-- ---------------------------------------------------------------------------
ALTER TABLE calendars         ENABLE ROW LEVEL SECURITY;
ALTER TABLE calendar_objects  ENABLE ROW LEVEL SECURITY;
ALTER TABLE events            ENABLE ROW LEVEL SECURITY;
ALTER TABLE attendees         ENABLE ROW LEVEL SECURITY;
ALTER TABLE invitations       ENABLE ROW LEVEL SECURITY;

ALTER TABLE calendars         FORCE ROW LEVEL SECURITY;
ALTER TABLE calendar_objects  FORCE ROW LEVEL SECURITY;
ALTER TABLE events            FORCE ROW LEVEL SECURITY;
ALTER TABLE attendees         FORCE ROW LEVEL SECURITY;
ALTER TABLE invitations       FORCE ROW LEVEL SECURITY;

CREATE POLICY calendars_tenant_isolation ON calendars
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY calobj_tenant_isolation ON calendar_objects
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY events_tenant_isolation ON events
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY attendees_tenant_isolation ON attendees
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY invitations_tenant_isolation ON invitations
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
