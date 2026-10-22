-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Etiquetas (labels) de usuario, estilo Gmail. En JMAP/IMAP se modelan como
-- KEYWORDS, no como buzones de multi-pertenencia: el esquema ya guarda los
-- keywords en messages.flags (text[]), donde un mensaje lleva N flags → N
-- etiquetas de forma natural, y SetFlags / el patch `keywords` de Email/set ya
-- los aplican. El TOKEN que ocupa la etiqueta en messages.flags es su id (uuid en
-- texto): estable, así renombrar/recolorear NO reescribe los mensajes, y es un
-- atom IMAP válido (solo [0-9a-f-]) y keyword JMAP válido. Esta tabla solo aporta
-- los metadatos que un keyword desnudo no tiene: nombre visible, color y dueño.
--
-- Multi-tenant por RLS (app.tenant) como el resto del esquema; además owner-scoped
-- por account_id en cada consulta (una etiqueta es de su cuenta, no del tenant).

CREATE TABLE mail_labels (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant      text        NOT NULL,
    account_id  uuid        NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    name        text        NOT NULL,
    -- Token de una paleta sobria (ver domain.LabelColors), no CSS crudo: alinea
    -- con la identidad, evita inyección de estilo y no es el único indicador (el
    -- nombre siempre acompaña). '' = sin color (neutro).
    color       text        NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT mail_labels_name_not_blank CHECK (length(name) > 0)
);

-- Nombre único (case-insensitive) por cuenta: no dos etiquetas «Trabajo».
CREATE UNIQUE INDEX mail_labels_unique_name ON mail_labels (account_id, lower(name));
CREATE INDEX mail_labels_account_idx ON mail_labels (tenant, account_id);

-- Membresía de etiqueta = keyword (id) presente en messages.flags. Este índice
-- GIN acelera `flags @> ARRAY['<id>']` tanto del listado filtrado por etiqueta
-- (keyset) como del recuento total/no-leído de cada etiqueta (sin N+1). Parcial
-- sobre no borrados: el listado y los contadores ignoran los soft-deleted.
CREATE INDEX messages_flags_gin_idx ON messages USING gin (flags) WHERE deleted_at IS NULL;

ALTER TABLE mail_labels ENABLE ROW LEVEL SECURITY;
ALTER TABLE mail_labels FORCE  ROW LEVEL SECURITY;

CREATE POLICY mail_labels_tenant_isolation ON mail_labels
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
