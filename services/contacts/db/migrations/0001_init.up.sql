-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Esquema base de Sytue Contacts: libretas de direcciones (colecciones CardDAV),
-- objetos de direccion (recursos vCard con su tarjeta canonica), contactos
-- normalizados (proyeccion indexable para la API REST y la busqueda), correos y
-- telefonos del contacto, y grupos. Estado en PostgreSQL (unica fuente de
-- verdad); sin Redis/Valkey ni motores AGPL.
--
-- Clean-room: modelo propio sobre los estandares abiertos vCard 4.0 (RFC 6350),
-- CardDAV (RFC 6352) y sincronizacion WebDAV (RFC 6578). Implementa la semantica
-- desde las especificaciones publicas; no deriva de codigo de terceros AGPL.
--
-- Multi-tenant: cada fila lleva `tenant` (slug). RLS deny-by-default aisla por la
-- variable de sesion `app.tenant` (la fija pg.SetTenant en cada transaccion).

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ---------------------------------------------------------------------------
-- Libretas de direcciones: una coleccion CardDAV (RFC 6352) por usuario/tenant.
-- ctag/sync soportan la sincronizacion incremental (RFC 6578). owner_sub es el
-- sujeto (claim "sub") de Sytue ID propietario de la coleccion.
-- ---------------------------------------------------------------------------
CREATE TABLE address_books (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant          text        NOT NULL,
    owner_sub       text        NOT NULL,
    -- name es el segmento de ruta CardDAV (URL-safe, estable). Unico por dueno.
    name            text        NOT NULL,
    display_name    text        NOT NULL DEFAULT '',
    description     text        NOT NULL DEFAULT '',
    -- color de presentacion (#RRGGBB[AA]) — propiedad CardDAV addressbook-color.
    color           text        NOT NULL DEFAULT '#3B82F6',
    is_default      boolean     NOT NULL DEFAULT false,
    -- ctag cambia con cualquier modificacion del contenido (getctag CardDAV).
    ctag            uuid        NOT NULL DEFAULT gen_random_uuid(),
    -- sync_token monotono para REPORT sync-collection (RFC 6578).
    sync_token      bigint      NOT NULL DEFAULT 1,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    deleted_at      timestamptz,
    CONSTRAINT abook_name_not_blank CHECK (length(name) > 0),
    CONSTRAINT abook_color_format CHECK (color ~ '^#[0-9A-Fa-f]{6}([0-9A-Fa-f]{2})?$')
);

-- Nombre de coleccion unico por dueno (case-insensitive) entre las vivas.
CREATE UNIQUE INDEX abook_unique_name
    ON address_books (tenant, owner_sub, lower(name))
    WHERE deleted_at IS NULL;

-- Una unica libreta por defecto por dueno.
CREATE UNIQUE INDEX abook_one_default
    ON address_books (tenant, owner_sub)
    WHERE is_default AND deleted_at IS NULL;

CREATE INDEX abook_owner_idx ON address_books (tenant, owner_sub) WHERE deleted_at IS NULL;

-- ---------------------------------------------------------------------------
-- Objetos de direccion: el RECURSO CardDAV (RFC 6352). Guardamos la vCard
-- canonica (data) para servirla verbatim por CardDAV y el etag para operaciones
-- condicionales (If-Match / If-None-Match). Un objeto = una tarjeta = un
-- contacto (o un grupo, KIND=group).
-- ---------------------------------------------------------------------------
CREATE TABLE card_objects (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant          text        NOT NULL,
    address_book_id uuid        NOT NULL REFERENCES address_books (id) ON DELETE CASCADE,
    -- uid es el UID de vCard (RFC 6350 6.7.6), unico dentro de la libreta.
    uid             text        NOT NULL,
    -- resource_name es el nombre de fichero del recurso CardDAV (suele ser
    -- "<uid>.vcf"); identifica el recurso en la URL de la coleccion.
    resource_name   text        NOT NULL,
    -- kind es el KIND de la vCard (RFC 6350 6.1.4): individual | group | org | ...
    kind            text        NOT NULL DEFAULT 'individual'
                        CHECK (kind IN ('individual', 'group', 'org', 'location', 'application', 'device')),
    -- data es la vCard 4.0 serializada canonica.
    data            text        NOT NULL,
    -- etag valida operaciones condicionales; cambia en cada escritura.
    etag            uuid        NOT NULL DEFAULT gen_random_uuid(),
    -- size en bytes del recurso (getcontentlength CardDAV).
    size_bytes      integer     NOT NULL DEFAULT 0,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    deleted_at      timestamptz,
    CONSTRAINT card_uid_not_blank CHECK (length(uid) > 0),
    CONSTRAINT card_resource_not_blank CHECK (length(resource_name) > 0)
);

-- UID unico por libreta (entre objetos vivos).
CREATE UNIQUE INDEX card_unique_uid
    ON card_objects (tenant, address_book_id, uid)
    WHERE deleted_at IS NULL;

-- resource_name unico por libreta (la URL del recurso CardDAV).
CREATE UNIQUE INDEX card_unique_resource
    ON card_objects (tenant, address_book_id, resource_name)
    WHERE deleted_at IS NULL;

CREATE INDEX card_abook_idx ON card_objects (tenant, address_book_id) WHERE deleted_at IS NULL;

-- ---------------------------------------------------------------------------
-- Contactos: proyeccion normalizada de la vCard de un objeto, para la API REST,
-- listados y busqueda. El objeto CardDAV (card_objects.data) sigue siendo la
-- fuente serializada; esta tabla es el indice consultable. KIND=group se modela
-- en contact_groups (mas abajo), no aqui.
-- ---------------------------------------------------------------------------
CREATE TABLE contacts (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant          text        NOT NULL,
    address_book_id uuid        NOT NULL REFERENCES address_books (id) ON DELETE CASCADE,
    object_id       uuid        NOT NULL REFERENCES card_objects (id) ON DELETE CASCADE,
    uid             text        NOT NULL,
    -- FN: nombre formateado para mostrar (RFC 6350 6.2.1), obligatorio en vCard.
    formatted_name  text        NOT NULL DEFAULT '',
    -- Componentes de N (RFC 6350 6.2.2): apellidos; nombre; segundos nombres;
    -- prefijos; sufijos. Se guardan desglosados para ordenar/buscar.
    family_name     text        NOT NULL DEFAULT '',
    given_name      text        NOT NULL DEFAULT '',
    additional_name text        NOT NULL DEFAULT '',
    honorific_prefix text       NOT NULL DEFAULT '',
    honorific_suffix text       NOT NULL DEFAULT '',
    -- ORG / TITLE (RFC 6350 6.6.4 / 6.6.1).
    organization    text        NOT NULL DEFAULT '',
    title           text        NOT NULL DEFAULT '',
    -- NICKNAME (RFC 6350 6.2.3), NOTE (6.7.2), BDAY (6.2.5).
    nickname        text        NOT NULL DEFAULT '',
    note            text        NOT NULL DEFAULT '',
    birthday        date,
    -- URL de foto (PHOTO, RFC 6350 6.2.4) — referencia, no el binario.
    photo_url       text        NOT NULL DEFAULT '',
    -- Vector tsvector de busqueda full-text (lo mantiene un trigger).
    search          tsvector,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    deleted_at      timestamptz
);

-- Un unico contacto por objeto de tarjeta vivo.
CREATE UNIQUE INDEX contacts_per_object
    ON contacts (tenant, object_id)
    WHERE deleted_at IS NULL;

CREATE INDEX contacts_abook_idx ON contacts (tenant, address_book_id) WHERE deleted_at IS NULL;
CREATE INDEX contacts_uid_idx ON contacts (tenant, uid) WHERE deleted_at IS NULL;
-- Orden de listado canonico (apellido, nombre, FN).
CREATE INDEX contacts_sort_idx
    ON contacts (tenant, address_book_id, lower(family_name), lower(given_name), lower(formatted_name))
    WHERE deleted_at IS NULL;
-- Busqueda full-text.
CREATE INDEX contacts_search_idx ON contacts USING gin (search) WHERE deleted_at IS NULL;

-- Trigger que mantiene el tsvector de busqueda a partir de los campos legibles.
CREATE OR REPLACE FUNCTION contacts_search_update() RETURNS trigger AS $$
BEGIN
    NEW.search :=
        setweight(to_tsvector('simple', coalesce(NEW.formatted_name, '')), 'A') ||
        setweight(to_tsvector('simple', coalesce(NEW.family_name, '')), 'A') ||
        setweight(to_tsvector('simple', coalesce(NEW.given_name, '')), 'A') ||
        setweight(to_tsvector('simple', coalesce(NEW.nickname, '')), 'B') ||
        setweight(to_tsvector('simple', coalesce(NEW.organization, '')), 'B') ||
        setweight(to_tsvector('simple', coalesce(NEW.title, '')), 'C') ||
        setweight(to_tsvector('simple', coalesce(NEW.note, '')), 'D');
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER contacts_search_trg
    BEFORE INSERT OR UPDATE ON contacts
    FOR EACH ROW EXECUTE FUNCTION contacts_search_update();

-- ---------------------------------------------------------------------------
-- Correos del contacto (EMAIL, RFC 6350 6.4.2). Un contacto puede tener varios;
-- type_label conserva el TYPE (home/work/...) y is_primary marca el PREF=1.
-- ---------------------------------------------------------------------------
CREATE TABLE contact_emails (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant          text        NOT NULL,
    contact_id      uuid        NOT NULL REFERENCES contacts (id) ON DELETE CASCADE,
    email           text        NOT NULL,
    type_label      text        NOT NULL DEFAULT 'home',
    is_primary      boolean     NOT NULL DEFAULT false,
    position        integer     NOT NULL DEFAULT 0,
    CONSTRAINT email_not_blank CHECK (length(email) > 0)
);

CREATE INDEX contact_emails_contact_idx ON contact_emails (tenant, contact_id);
CREATE INDEX contact_emails_addr_idx ON contact_emails (tenant, lower(email));

-- ---------------------------------------------------------------------------
-- Telefonos del contacto (TEL, RFC 6350 6.4.1). Igual modelo que los correos.
-- ---------------------------------------------------------------------------
CREATE TABLE contact_phones (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant          text        NOT NULL,
    contact_id      uuid        NOT NULL REFERENCES contacts (id) ON DELETE CASCADE,
    phone           text        NOT NULL,
    type_label      text        NOT NULL DEFAULT 'cell',
    is_primary      boolean     NOT NULL DEFAULT false,
    position        integer     NOT NULL DEFAULT 0,
    CONSTRAINT phone_not_blank CHECK (length(phone) > 0)
);

CREATE INDEX contact_phones_contact_idx ON contact_phones (tenant, contact_id);

-- ---------------------------------------------------------------------------
-- Grupos: una vCard con KIND=group (RFC 6350 6.1.4) cuyos MEMBER (6.6.5) son los
-- UID de los contactos miembros. Modelamos el grupo como su propio objeto de
-- tarjeta (group_object_id) y la pertenencia en contact_group_members.
-- ---------------------------------------------------------------------------
CREATE TABLE contact_groups (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant          text        NOT NULL,
    address_book_id uuid        NOT NULL REFERENCES address_books (id) ON DELETE CASCADE,
    -- objeto de tarjeta (KIND=group) que respalda al grupo en CardDAV.
    object_id       uuid        NOT NULL REFERENCES card_objects (id) ON DELETE CASCADE,
    uid             text        NOT NULL,
    name            text        NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    deleted_at      timestamptz,
    CONSTRAINT group_name_not_blank CHECK (length(name) > 0)
);

CREATE UNIQUE INDEX groups_per_object
    ON contact_groups (tenant, object_id)
    WHERE deleted_at IS NULL;
CREATE INDEX groups_abook_idx ON contact_groups (tenant, address_book_id) WHERE deleted_at IS NULL;
CREATE INDEX groups_uid_idx ON contact_groups (tenant, uid) WHERE deleted_at IS NULL;

-- Pertenencia: un contacto pertenece a un grupo (MEMBER por UID en la vCard del
-- grupo). Guardamos contact_uid para resolver miembros aun antes de existir el
-- contacto local (CardDAV permite MEMBER colgantes).
CREATE TABLE contact_group_members (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant          text        NOT NULL,
    group_id        uuid        NOT NULL REFERENCES contact_groups (id) ON DELETE CASCADE,
    contact_uid     text        NOT NULL,
    -- contact_id resuelto (si el miembro existe como contacto local).
    contact_id      uuid        REFERENCES contacts (id) ON DELETE SET NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT member_uid_not_blank CHECK (length(contact_uid) > 0)
);

CREATE UNIQUE INDEX group_member_unique
    ON contact_group_members (tenant, group_id, contact_uid);
CREATE INDEX group_member_group_idx ON contact_group_members (tenant, group_id);
CREATE INDEX group_member_contact_idx ON contact_group_members (tenant, contact_id);

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
ALTER TABLE address_books         ENABLE ROW LEVEL SECURITY;
ALTER TABLE card_objects          ENABLE ROW LEVEL SECURITY;
ALTER TABLE contacts              ENABLE ROW LEVEL SECURITY;
ALTER TABLE contact_emails        ENABLE ROW LEVEL SECURITY;
ALTER TABLE contact_phones        ENABLE ROW LEVEL SECURITY;
ALTER TABLE contact_groups        ENABLE ROW LEVEL SECURITY;
ALTER TABLE contact_group_members ENABLE ROW LEVEL SECURITY;

ALTER TABLE address_books         FORCE ROW LEVEL SECURITY;
ALTER TABLE card_objects          FORCE ROW LEVEL SECURITY;
ALTER TABLE contacts              FORCE ROW LEVEL SECURITY;
ALTER TABLE contact_emails        FORCE ROW LEVEL SECURITY;
ALTER TABLE contact_phones        FORCE ROW LEVEL SECURITY;
ALTER TABLE contact_groups        FORCE ROW LEVEL SECURITY;
ALTER TABLE contact_group_members FORCE ROW LEVEL SECURITY;

CREATE POLICY abook_tenant_isolation ON address_books
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY card_tenant_isolation ON card_objects
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY contacts_tenant_isolation ON contacts
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY contact_emails_tenant_isolation ON contact_emails
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY contact_phones_tenant_isolation ON contact_phones
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY groups_tenant_isolation ON contact_groups
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY group_members_tenant_isolation ON contact_group_members
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
