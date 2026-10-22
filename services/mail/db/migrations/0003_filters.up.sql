-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Reglas/filtros de correo del lado servidor (estilo Sieve, RFC 5228, pero modelo
-- propio y minimalista): al entregar un mensaje entrante, la primera regla activa
-- cuyas condiciones casen aplica sus acciones (mover de carpeta, marcar leído,
-- destacar). Se evalúan en orden de `priority` ascendente (empate por creación).
--
-- Multi-tenant por RLS como el resto del esquema (app.tenant). Las reglas se
-- consultan bajo el tenant del destinatario tanto por la API JMAP como por la
-- entrega entrante (que fija el tenant tras resolver la dirección).

CREATE TABLE mail_filters (
    id            uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant        text        NOT NULL,
    account_id    uuid        NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    name          text        NOT NULL DEFAULT '',
    enabled       boolean     NOT NULL DEFAULT true,
    priority      int         NOT NULL DEFAULT 0,

    -- Condiciones (AND de las no vacías): subcadena case-insensitive.
    match_from    text        NOT NULL DEFAULT '',      -- remitente (dirección o nombre)
    match_subject text        NOT NULL DEFAULT '',      -- asunto

    -- Acciones.
    move_to       uuid        REFERENCES mailboxes (id) ON DELETE CASCADE, -- NULL = no mover
    mark_read     boolean     NOT NULL DEFAULT false,
    mark_flagged  boolean     NOT NULL DEFAULT false,

    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),

    -- Una regla sin ninguna condición casaría con todo el correo: se prohíbe.
    CONSTRAINT mail_filters_has_match CHECK (match_from <> '' OR match_subject <> '')
);

-- Camino caliente: reglas activas de una cuenta en orden de evaluación. Parcial
-- sobre enabled porque la entrega solo evalúa las activas.
CREATE INDEX mail_filters_active_idx
    ON mail_filters (tenant, account_id, priority, created_at) WHERE enabled;

ALTER TABLE mail_filters ENABLE ROW LEVEL SECURITY;
ALTER TABLE mail_filters FORCE  ROW LEVEL SECURITY;

CREATE POLICY mail_filters_tenant_isolation ON mail_filters
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
