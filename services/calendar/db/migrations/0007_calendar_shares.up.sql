-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Compartir calendarios con ACL: un dueño concede a otro usuario del tenant
-- acceso de LECTURA (reader) o EDICIÓN (writer) a uno de sus calendarios. La
-- comparticion se guarda por IDENTIDAD del destinatario —su correo verificado y/o
-- su sub de Sytue ID— y se hace cumplir al acceder casando contra los claims del
-- JWT ya verificado (email/sub del token), sin depender de un directorio
-- email→usuario: el share se crea por correo; cuando ese usuario entra, su email
-- (o sub) del token casa el share.
--
-- Enforcement (en la capa store/servicio): acceso a un calendario = dueño O share
-- vigente que case sub/email; sin coincidencia → deny-by-default (404, sin filtrar
-- existencia). Un reader no escribe (se rechaza con ErrReadOnly, como la puerta de
-- las suscripciones ICS). Solo el DUEÑO comparte/revoca. Aislamiento por tenant.

CREATE TABLE calendar_shares (
    id            uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant        text        NOT NULL,
    -- Calendario compartido; al borrarlo, sus shares caen en cascada.
    calendar_id   uuid        NOT NULL REFERENCES calendars (id) ON DELETE CASCADE,
    -- Identidad del destinatario: su correo (normalizado a minúsculas) y/o su sub.
    -- Se crea por correo (grantee_sub queda vacío hasta que se conozca); el gate
    -- casa cualquiera de los dos contra el JWT verificado.
    grantee_email text        NOT NULL DEFAULT '',
    grantee_sub   text        NOT NULL DEFAULT '',
    -- Nivel de acceso: 'reader' (ver) | 'writer' (ver + editar).
    permission    text        NOT NULL CHECK (permission IN ('reader', 'writer')),
    -- sub del dueño que compartió (para «compartido por» en «Compartido conmigo»).
    created_by    text        NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    -- Un share debe apuntar a alguna identidad (correo o sub).
    CONSTRAINT calendar_shares_grantee_present CHECK (grantee_email <> '' OR grantee_sub <> '')
);

-- Un único share por (calendario, destinatario): re-compartir con el mismo correo
-- actualiza el permiso (upsert por este índice). grantee_sub es '' al crear por
-- correo, así la unicidad es efectivamente por (calendario, correo).
CREATE UNIQUE INDEX calendar_shares_unique_grantee
    ON calendar_shares (tenant, calendar_id, grantee_email, grantee_sub);

-- «Shares de este calendario» (el dueño lista/revoca): recorrido indexado.
CREATE INDEX calendar_shares_calendar_idx
    ON calendar_shares (tenant, calendar_id);

-- «Calendarios compartidos con este destinatario» (shared-with-me + gate): por
-- correo y por sub, parciales para no indexar las filas con esa identidad vacía.
CREATE INDEX calendar_shares_email_idx
    ON calendar_shares (tenant, grantee_email)
    WHERE grantee_email <> '';
CREATE INDEX calendar_shares_sub_idx
    ON calendar_shares (tenant, grantee_sub)
    WHERE grantee_sub <> '';

-- RLS: aislamiento por tenant deny-by-default, como el resto del esquema. Los
-- shares nunca cruzan tenant (además, el FK a calendars ata el share al tenant del
-- calendario).
ALTER TABLE calendar_shares ENABLE ROW LEVEL SECURITY;
ALTER TABLE calendar_shares FORCE ROW LEVEL SECURITY;

CREATE POLICY calendar_shares_tenant_isolation ON calendar_shares
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
