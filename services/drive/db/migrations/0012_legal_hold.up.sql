-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Legal hold + eDiscovery para FICHEROS (paridad M365 Purview / Google Vault, la
-- pieza NO destructiva del compliance; espeja la ola de Mail, mig 0009). Dos tablas:
--
--   * legal_holds  — preservación de ficheros para litigios. Mientras un sujeto
--                    (un usuario por su `sub` = owner_sub, o TODA la org si subject
--                    IS NULL) tiene un hold ACTIVO (released_at IS NULL), sus
--                    ficheros y TODAS sus versiones NUNCA se borran físicamente: la
--                    papelera (soft-delete via files.deleted_at) los sigue ocultando,
--                    pero la purga (manual y auto-purga de papelera), la cascada de
--                    borrado de carpeta/unidad y —por cascada FK desde files— el
--                    borrado de versiones respetan el hold. Al liberar el hold, el
--                    fichero vuelve al ciclo normal (purgable). Preservación
--                    DINÁMICA: sin sello por-fichero; se evalúa contra los holds
--                    activos EN EL PUNTO de borrado (NOT EXISTS hold activo).
--
--   * audit_log    — traza de acceso privilegiado (crear/liberar hold, buscar y
--                    exportar en eDiscovery). Espeja el audit_log de Mail / services/id.
--
-- Multi-tenant: RLS deny-by-default por app.tenant (igual que folders/files). Un
-- admin NUNCA accede a ficheros/holds/traza de otra org.

CREATE TABLE legal_holds (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant      text        NOT NULL,
    subject     text,                                     -- `sub` de Sytue ID (owner_sub); NULL = toda la org
    reason      text        NOT NULL DEFAULT '',
    case_ref    text        NOT NULL DEFAULT '',          -- identificador del caso/litigio
    created_by  text        NOT NULL,                     -- sub del admin que lo creó
    created_at  timestamptz NOT NULL DEFAULT now(),
    released_at timestamptz,                              -- NULL = activo
    released_by text        NOT NULL DEFAULT ''
);

-- Holds ACTIVOS que cubren a un sujeto (o a toda la org): los consultan la purga
-- (NOT EXISTS) y la auto-purga (skip del claim).
CREATE INDEX legal_holds_active_idx ON legal_holds (tenant, subject) WHERE released_at IS NULL;
CREATE INDEX legal_holds_tenant_idx ON legal_holds (tenant, created_at DESC);

-- Traza de auditoría de acceso privilegiado a ficheros ajenos (compliance).
CREATE TABLE audit_log (
    id          bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    occurred_at timestamptz NOT NULL DEFAULT now(),
    tenant      text        NOT NULL,
    actor       text        NOT NULL,                     -- sub del admin/owner
    action      text        NOT NULL,                     -- legalhold.create|legalhold.release|ediscovery.search|ediscovery.export
    target      text        NOT NULL DEFAULT '',          -- sujeto objetivo (o '' si org-wide)
    detail      jsonb       NOT NULL DEFAULT '{}'::jsonb  -- caso, consulta, filtros, nº de resultados
);
CREATE INDEX audit_log_tenant_idx ON audit_log (tenant, occurred_at DESC);

-- ---------------------------------------------------------------------------
-- RLS: aislamiento por tenant deny-by-default (CONVENTIONS §10), como folders/files.
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

-- Política PERMISSIVE de mantenimiento sobre legal_holds (se OR-ea con la de
-- aislamiento), como folders/files en la mig 0009. La AUTO-PURGA de papelera es un
-- worker CROSS-TENANT (InMaintenanceTx: fija app.maintenance, NO app.tenant) que
-- debe LEER los holds para saltarse los ficheros preservados en su claim; sin esta
-- política los vería como CERO holds y purgaría ficheros bajo hold (pérdida de
-- datos). El worker solo LEE holds; el aislamiento lo mantiene el predicado
-- h.tenant = <tenant de la fila> de cada consulta de purga. eDiscovery también lee
-- ficheros cross-ámbito (Mi unidad + unidades de equipo) bajo esta vía.
CREATE POLICY legal_holds_maintenance ON legal_holds
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');
