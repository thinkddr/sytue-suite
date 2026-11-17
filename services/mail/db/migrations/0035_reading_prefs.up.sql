-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- LECTURA: a dónde se va tras archivar, qué hace deslizar una fila en el móvil y
-- de qué remitentes se enseñan las imágenes sin preguntar. Por cuenta y en el
-- servidor, como la ventana de deshacer (0020): son costumbres de quien lee, no del
-- equipo desde el que lee.
--
-- Auto-avance y gestos: columnas de `accounts` (un escalar con la vida de la cuenta;
-- `accounts` ya va con RLS desde 0001). Los valores por defecto son los del plan:
-- tras archivar se abre la conversación SIGUIENTE (la más antigua), deslizar a la
-- derecha archiva y a la izquierda manda a la papelera. Los CHECK son la última
-- línea: el servicio ya rechaza lo demás con un 400.
--
-- Remitentes de confianza: TABLA, son N por cuenta. La dirección se guarda en
-- minúsculas y sin espacios (la normaliza el servicio): «Ana@X.eu» y «ana@x.eu» son
-- el mismo remitente. Se confía en la DIRECCIÓN, no en el dominio: confiar en un
-- boletín de gmail.com no debe abrir las imágenes de cualquiera de gmail.com.

ALTER TABLE accounts
  ADD COLUMN auto_advance text NOT NULL DEFAULT 'older'
    CONSTRAINT accounts_auto_advance_check CHECK (auto_advance IN ('older', 'newer', 'list')),
  ADD COLUMN swipe_right text NOT NULL DEFAULT 'archive'
    CONSTRAINT accounts_swipe_right_check CHECK (swipe_right IN ('archive', 'trash', 'read', 'none')),
  ADD COLUMN swipe_left text NOT NULL DEFAULT 'trash'
    CONSTRAINT accounts_swipe_left_check CHECK (swipe_left IN ('archive', 'trash', 'read', 'none'));

CREATE TABLE mail_trusted_senders (
    tenant      text        NOT NULL,
    account_id  uuid        NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    address     text        NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (account_id, address),

    CONSTRAINT mail_trusted_senders_address_lower CHECK (address = lower(btrim(address))),
    CONSTRAINT mail_trusted_senders_address_len CHECK (char_length(address) BETWEEN 3 AND 320)
);

CREATE INDEX mail_trusted_senders_account_idx ON mail_trusted_senders (tenant, account_id);

ALTER TABLE mail_trusted_senders ENABLE ROW LEVEL SECURITY;
ALTER TABLE mail_trusted_senders FORCE  ROW LEVEL SECURITY;

CREATE POLICY mail_trusted_senders_tenant_isolation ON mail_trusted_senders
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

-- La de fusión de organizaciones (ver 0010), como en mail_templates (0021).
CREATE POLICY mail_trusted_senders_platform_merge ON mail_trusted_senders
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
