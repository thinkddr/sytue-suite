-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Esquema base de Sytue Mail: cuentas, buzones (carpetas), mensajes (sobres),
-- cola de salida y outbox de eventos. Metadatos en PostgreSQL (única fuente de
-- verdad); los cuerpos MIME crudos y adjuntos viven en Object Storage S3.
--
-- Multi-tenant: cada fila lleva `tenant` (slug). RLS deny-by-default aísla por
-- la variable de sesión `app.tenant` (la fija pg.SetTenant en cada transacción).
--
-- Clean-room: modelo propio sobre RFC 5321/5322 (SMTP/IMF), RFC 3501/9051
-- (IMAP UID/UIDVALIDITY), RFC 8620/8621 (JMAP) y RFC 8601 (Authentication-Results).

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ---------------------------------------------------------------------------
-- Cuentas: una por identidad de Sytue ID (sub) dentro de un tenant.
-- ---------------------------------------------------------------------------
CREATE TABLE accounts (
    id            uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant        text        NOT NULL,
    subject       text        NOT NULL,                 -- sub de Sytue ID
    address       text        NOT NULL,                 -- dirección principal (local@domain)
    display_name  text        NOT NULL DEFAULT '',
    quota_bytes   bigint      NOT NULL DEFAULT 0 CHECK (quota_bytes >= 0),
    used_bytes    bigint      NOT NULL DEFAULT 0 CHECK (used_bytes >= 0),
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX accounts_tenant_subject_idx ON accounts (tenant, subject);
CREATE UNIQUE INDEX accounts_address_idx        ON accounts (lower(address));

-- Alias adicionales que entregan al mismo buzón (cuenta).
CREATE TABLE aliases (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant      text        NOT NULL,
    account_id  uuid        NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    address     text        NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX aliases_address_idx ON aliases (lower(address));
CREATE INDEX aliases_account_idx ON aliases (tenant, account_id);

-- ---------------------------------------------------------------------------
-- Buzones (carpetas IMAP / Mailbox JMAP). UIDVALIDITY/UIDNEXT por buzón.
-- ---------------------------------------------------------------------------
CREATE TABLE mailboxes (
    id            uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant        text        NOT NULL,
    account_id    uuid        NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    parent_id     uuid        REFERENCES mailboxes (id) ON DELETE CASCADE,
    name          text        NOT NULL,
    role          text        NOT NULL DEFAULT '',      -- inbox|sent|drafts|trash|junk|archive|''
    uid_validity  bigint      NOT NULL DEFAULT (extract(epoch from now())::bigint),
    uid_next      bigint      NOT NULL DEFAULT 1 CHECK (uid_next >= 1),
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT mailboxes_name_not_blank CHECK (length(name) > 0)
);

-- Nombre único entre hermanos del mismo buzón/cuenta (case-insensitive).
CREATE UNIQUE INDEX mailboxes_unique_sibling_name
    ON mailboxes (account_id, COALESCE(parent_id, '00000000-0000-0000-0000-000000000000'::uuid), lower(name));
-- Un único buzón por rol especial en cada cuenta.
CREATE UNIQUE INDEX mailboxes_unique_role
    ON mailboxes (account_id, role) WHERE role <> '';
CREATE INDEX mailboxes_account_idx ON mailboxes (tenant, account_id);

-- ---------------------------------------------------------------------------
-- Hilos: agrupan mensajes de una conversación (References/In-Reply-To).
-- ---------------------------------------------------------------------------
CREATE TABLE threads (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant      text        NOT NULL,
    account_id  uuid        NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    subject     text        NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX threads_account_idx ON threads (tenant, account_id, updated_at DESC);

-- ---------------------------------------------------------------------------
-- Mensajes (sobre/metadato). Cuerpo MIME crudo y adjuntos en S3 (blob_key).
-- uid es estable dentro del buzón (semántica IMAP). flags como text[].
-- ---------------------------------------------------------------------------
CREATE TABLE messages (
    id            uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant        text        NOT NULL,
    account_id    uuid        NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    mailbox_id    uuid        NOT NULL REFERENCES mailboxes (id) ON DELETE CASCADE,
    thread_id     uuid        REFERENCES threads (id) ON DELETE SET NULL,
    uid           bigint      NOT NULL,                 -- UID IMAP dentro del buzón
    rfc822_msg_id text        NOT NULL DEFAULT '',      -- Message-ID RFC 5322
    in_reply_to   text        NOT NULL DEFAULT '',

    subject       text        NOT NULL DEFAULT '',
    from_addr     text        NOT NULL DEFAULT '',
    from_name     text        NOT NULL DEFAULT '',
    to_addrs      text[]      NOT NULL DEFAULT '{}',
    cc_addrs      text[]      NOT NULL DEFAULT '{}',
    bcc_addrs     text[]      NOT NULL DEFAULT '{}',
    reply_to      text[]      NOT NULL DEFAULT '{}',

    snippet       text        NOT NULL DEFAULT '',
    blob_key      text        NOT NULL,                 -- clave S3 del .eml crudo
    size_bytes    bigint      NOT NULL DEFAULT 0 CHECK (size_bytes >= 0),
    flags         text[]      NOT NULL DEFAULT '{}',
    has_attach    boolean     NOT NULL DEFAULT false,

    spf_result    text        NOT NULL DEFAULT '',
    dkim_result   text        NOT NULL DEFAULT '',
    dmarc_result  text        NOT NULL DEFAULT '',

    received_at   timestamptz NOT NULL DEFAULT now(),
    sent_at       timestamptz,
    deleted_at    timestamptz
);

CREATE UNIQUE INDEX messages_mailbox_uid_idx ON messages (mailbox_id, uid);
CREATE INDEX messages_mailbox_idx  ON messages (tenant, mailbox_id, received_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX messages_thread_idx   ON messages (tenant, thread_id) WHERE deleted_at IS NULL;
CREATE INDEX messages_unread_idx   ON messages (mailbox_id) WHERE NOT (flags @> ARRAY['\Seen']) AND deleted_at IS NULL;
CREATE INDEX messages_rfc822_idx   ON messages (tenant, account_id, rfc822_msg_id) WHERE rfc822_msg_id <> '';
-- Búsqueda full-text básica sobre asunto + remitente (FTS nativo de Postgres).
CREATE INDEX messages_fts_idx ON messages
    USING gin (to_tsvector('simple', coalesce(subject,'') || ' ' || coalesce(from_addr,'') || ' ' || coalesce(snippet,'')));

-- ---------------------------------------------------------------------------
-- Cola de SALIDA (sobre PostgreSQL; sin Redis/Valkey). Reintento con backoff.
-- SELECT ... FOR UPDATE SKIP LOCKED para reparto entre workers.
-- ---------------------------------------------------------------------------
CREATE TABLE outbound (
    id            uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant        text        NOT NULL,
    account_id    uuid        NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    message_id    uuid        REFERENCES messages (id) ON DELETE SET NULL,
    mail_from     text        NOT NULL,                 -- envelope return-path
    recipients    text[]      NOT NULL,                 -- envelope RCPT TO de este lote
    blob_key      text        NOT NULL,                 -- .eml firmado en S3
    state         text        NOT NULL DEFAULT 'queued'
                  CHECK (state IN ('queued','sending','sent','deferred','bounced','cancelled')),
    attempts      int         NOT NULL DEFAULT 0,
    last_error    text        NOT NULL DEFAULT '',
    next_retry_at timestamptz NOT NULL DEFAULT now(),
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

-- Índice de claim: mensajes drenables ordenados por reintento.
CREATE INDEX outbound_claim_idx ON outbound (next_retry_at)
    WHERE state IN ('queued','deferred');
CREATE INDEX outbound_tenant_idx ON outbound (tenant, account_id, created_at DESC);

-- ---------------------------------------------------------------------------
-- Rate-limit de recepción por IP (ventana deslizante minuto a minuto), sobre
-- Postgres (sin Redis/Valkey). Anti-abuso del MX entrante.
-- ---------------------------------------------------------------------------
CREATE TABLE inbound_rate (
    ip          inet        NOT NULL,
    window_min  timestamptz NOT NULL,                   -- truncado a minuto
    count       int         NOT NULL DEFAULT 0,
    PRIMARY KEY (ip, window_min)
);
CREATE INDEX inbound_rate_window_idx ON inbound_rate (window_min);

-- ---------------------------------------------------------------------------
-- Outbox de eventos (sobre PostgreSQL; sin Redis/Valkey). DDL compartido de
-- platform/pkg/events, replicado porque cada servicio gestiona su esquema.
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
    ON events_outbox (created_at) WHERE processed_at IS NULL;

-- ---------------------------------------------------------------------------
-- Row-Level Security: aislamiento por tenant deny-by-default (CONVENTIONS §10).
-- ---------------------------------------------------------------------------
ALTER TABLE accounts     ENABLE ROW LEVEL SECURITY;
ALTER TABLE aliases      ENABLE ROW LEVEL SECURITY;
ALTER TABLE mailboxes    ENABLE ROW LEVEL SECURITY;
ALTER TABLE threads      ENABLE ROW LEVEL SECURITY;
ALTER TABLE messages     ENABLE ROW LEVEL SECURITY;
ALTER TABLE outbound     ENABLE ROW LEVEL SECURITY;

ALTER TABLE accounts     FORCE ROW LEVEL SECURITY;
ALTER TABLE aliases      FORCE ROW LEVEL SECURITY;
ALTER TABLE mailboxes    FORCE ROW LEVEL SECURITY;
ALTER TABLE threads      FORCE ROW LEVEL SECURITY;
ALTER TABLE messages     FORCE ROW LEVEL SECURITY;
ALTER TABLE outbound     FORCE ROW LEVEL SECURITY;

CREATE POLICY accounts_tenant_isolation ON accounts
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY aliases_tenant_isolation ON aliases
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY mailboxes_tenant_isolation ON mailboxes
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY threads_tenant_isolation ON threads
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY messages_tenant_isolation ON messages
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY outbound_tenant_isolation ON outbound
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

-- Políticas de MANTENIMIENTO (PERMISSIVE, se suman a las de aislamiento): habilitan
-- las operaciones cross-tenant legítimas del servicio SOLO cuando la sesión declara
-- explícitamente `app.maintenance='on'` (la fija pg.SetMaintenance / InMaintenanceTx):
--   * MX entrante: resolver dirección -> cuenta/tenant ANTES de conocer el tenant.
--   * Drenaje de la cola de salida: el worker procesa outbound de todos los tenants.
-- De este modo NO se requiere un rol BYPASSRLS (que anularía RLS también para la API
-- JMAP): el mismo rol de aplicación mantiene el aislamiento por tenant en el camino
-- de negocio (JMAP/IMAP con app.tenant) y solo abre el acceso cross-tenant cuando el
-- código lo declara. Las transacciones de negocio nunca fijan app.maintenance.
CREATE POLICY accounts_maintenance ON accounts
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');
CREATE POLICY aliases_maintenance ON aliases
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');
CREATE POLICY outbound_maintenance ON outbound
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');

-- NOTA: accounts.address y aliases.address son globales (entrega entrante por
-- dirección, antes de conocer el tenant). El MX entrante y el worker de cola
-- resuelven/operan cross-tenant con `app.maintenance='on'` (política PERMISSIVE
-- de arriba), NO con un rol BYPASSRLS. La API JMAP/IMAP siempre opera con RLS
-- activo bajo el tenant del token de Sytue ID (sin el flag de mantenimiento).
