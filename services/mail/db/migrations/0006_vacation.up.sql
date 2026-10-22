-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Autorespuesta / vacaciones (respuesta automática de fuera-de-oficina). Config
-- por CUENTA (una fila por account_id) + bitácora de dedupe por remitente para no
-- responder más de una vez por ventana (RFC 3834: evitar bucles y tormentas).
--
-- La tabla de aliases (direcciones adicionales que entregan a la misma cuenta) ya
-- existe desde 0001_init; esta migración solo añade lo de vacaciones.
--
-- Multi-tenant por RLS (app.tenant) como el resto del esquema; además owner-scoped
-- por account_id. La entrega entrante ya corre bajo el tenant del destinatario, así
-- que basta la política de aislamiento por tenant (sin política de mantenimiento).

-- Configuración de vacaciones de la cuenta (una fila por cuenta; upsert).
CREATE TABLE mail_vacation (
    account_id  uuid        PRIMARY KEY REFERENCES accounts (id) ON DELETE CASCADE,
    tenant      text        NOT NULL,
    enabled     boolean     NOT NULL DEFAULT false,
    subject     text        NOT NULL DEFAULT '',
    body        text        NOT NULL DEFAULT '',
    -- Rango de actividad opcional (NULL = sin límite por ese extremo). Fuera del
    -- rango no se responde aunque enabled sea true.
    starts_at   timestamptz,
    ends_at     timestamptz,
    updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX mail_vacation_tenant_idx ON mail_vacation (tenant, account_id);

-- Bitácora de autorespuestas enviadas, para dedupe por remitente (RFC 3834): no
-- responder al mismo remitente más de una vez cada N días. Una fila por
-- (cuenta, remitente) con la última fecha de respuesta; el claim atómico
-- (INSERT … ON CONFLICT DO UPDATE … WHERE last_sent_at < now()-ventana) decide y
-- registra el envío en una sola operación (exactly-once por ventana, sin carrera).
CREATE TABLE mail_vacation_log (
    tenant        text        NOT NULL,
    account_id    uuid        NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    sender        text        NOT NULL,                  -- remitente (envelope) en minúsculas
    last_sent_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (account_id, sender)                     -- índice por (cuenta, remitente)
);

ALTER TABLE mail_vacation     ENABLE ROW LEVEL SECURITY;
ALTER TABLE mail_vacation_log ENABLE ROW LEVEL SECURITY;
ALTER TABLE mail_vacation     FORCE  ROW LEVEL SECURITY;
ALTER TABLE mail_vacation_log FORCE  ROW LEVEL SECURITY;

CREATE POLICY mail_vacation_tenant_isolation ON mail_vacation
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY mail_vacation_log_tenant_isolation ON mail_vacation_log
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
