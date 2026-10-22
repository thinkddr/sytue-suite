-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Legal hold + eDiscovery (paridad M365 Purview / Google Vault, pieza NO
-- destructiva del compliance). Dos tablas nuevas:
--
--   * legal_holds  — preservación de correo para litigios. Mientras un sujeto
--                    (un usuario, o toda la org si subject IS NULL) tiene un hold
--                    ACTIVO (released_at IS NULL), su correo NUNCA se purga de
--                    verdad: el borrado normal (soft-delete via messages.deleted_at)
--                    lo sigue ocultando de la bandeja, pero la purga física
--                    (store.PurgeDeleted) y el cascade de borrado de carpeta lo
--                    respetan. Al liberar el hold el correo vuelve al ciclo normal.
--
--   * audit_log    — traza de acceso privilegiado (crear/liberar hold, buscar y
--                    exportar en eDiscovery). Espeja el audit_log de services/id.
--
-- Multi-tenant: RLS deny-by-default por app.tenant (igual que el resto del
-- esquema de Mail). Un admin NUNCA accede a correo/holds/traza de otra org.

CREATE TABLE legal_holds (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant      text        NOT NULL,
    subject     text,                                     -- sub de Sytue ID; NULL = toda la org
    reason      text        NOT NULL DEFAULT '',
    case_ref    text        NOT NULL DEFAULT '',          -- identificador del caso/litigio
    created_by  text        NOT NULL,                     -- sub del admin que lo creó
    created_at  timestamptz NOT NULL DEFAULT now(),
    released_at timestamptz,                              -- NULL = activo
    released_by text        NOT NULL DEFAULT ''
);

-- Búsqueda de holds ACTIVOS que cubren a un sujeto (o a toda la org): la usan
-- PurgeDeleted (NOT EXISTS) y el cascade de DeleteMailbox (EXISTS).
CREATE INDEX legal_holds_active_idx ON legal_holds (tenant, subject) WHERE released_at IS NULL;
CREATE INDEX legal_holds_tenant_idx ON legal_holds (tenant, created_at DESC);

-- Traza de auditoría de acceso privilegiado a correo ajeno (compliance).
CREATE TABLE audit_log (
    id          bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    occurred_at timestamptz NOT NULL DEFAULT now(),
    tenant      text        NOT NULL,
    actor       text        NOT NULL,                     -- sub del admin/owner
    action      text        NOT NULL,                     -- legalhold.create|legalhold.release|ediscovery.search|ediscovery.export
    target      text        NOT NULL DEFAULT '',          -- sujeto/email objetivo (o '' si org-wide)
    detail      jsonb       NOT NULL DEFAULT '{}'::jsonb  -- caso, consulta, rango de fechas, nº de resultados
);
CREATE INDEX audit_log_tenant_idx ON audit_log (tenant, occurred_at DESC);

-- ---------------------------------------------------------------------------
-- RLS: aislamiento por tenant deny-by-default (CONVENTIONS §10), como el resto
-- del esquema. Sin política de mantenimiento: legal hold y eDiscovery corren
-- SIEMPRE bajo el tenant del admin (InTenantTx), nunca cross-tenant.
-- ---------------------------------------------------------------------------
ALTER TABLE legal_holds ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_log   ENABLE ROW LEVEL SECURITY;
ALTER TABLE legal_holds FORCE ROW LEVEL SECURITY;
ALTER TABLE audit_log   FORCE ROW LEVEL SECURITY;

CREATE POLICY legal_holds_tenant_isolation ON legal_holds
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY audit_log_tenant_isolation ON audit_log
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
