-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Conector de datos externos de Sytue Sheets (estilo Power Query): conexiones a
-- orígenes externos (REST autenticada, PostgreSQL) y consultas guardadas que se
-- materializan como tabla en la hoja y se refrescan.
--
-- Seguridad: las CREDENCIALES (api-key/token/contraseña) se guardan CIFRADAS
-- at-rest en la columna bytea `credentials` (AES-256-GCM, clave por env; ver
-- internal/connectors.Sealer); NUNCA en claro, NUNCA se devuelven al cliente. La
-- config pública (jsonb) no lleva secretos. Aislamiento multi-tenant por RLS
-- deny-by-default (columna `tenant` + current_setting('app.tenant')), igual que
-- el resto del esquema (0001_init).

-- ---------------------------------------------------------------------------
-- data_connections — conexión a un origen externo (config pública + credencial
-- cifrada). kind: 'rest' | 'postgres'.
-- ---------------------------------------------------------------------------
CREATE TABLE data_connections (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant       text        NOT NULL,
    name         text        NOT NULL,
    kind         text        NOT NULL,
    -- Configuración PÚBLICA (sin secretos): base_url/auth (REST), host/db/user
    -- (postgres). El secreto va en `credentials`, cifrado.
    config       jsonb       NOT NULL DEFAULT '{}'::jsonb,
    -- Credencial CIFRADA (nonce||ciphertext||tag de AES-256-GCM). NULL si la
    -- conexión no requiere secreto. Jamás en claro.
    credentials  bytea,
    created_by   text        NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT data_connections_kind_chk CHECK (kind IN ('rest', 'postgres'))
);

CREATE INDEX data_connections_tenant_idx ON data_connections (tenant, name);

-- ---------------------------------------------------------------------------
-- data_queries — consulta guardada sobre una conexión (spec = QuerySpec jsonb).
-- ON DELETE CASCADE: al borrar la conexión se borran sus consultas.
-- ---------------------------------------------------------------------------
CREATE TABLE data_queries (
    id            uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant        text        NOT NULL,
    connection_id uuid        NOT NULL REFERENCES data_connections (id) ON DELETE CASCADE,
    name          text        NOT NULL,
    spec          jsonb       NOT NULL DEFAULT '{}'::jsonb,
    created_by    text        NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX data_queries_conn_idx ON data_queries (tenant, connection_id);

-- ---------------------------------------------------------------------------
-- Row-Level Security (multi-tenant, deny-by-default) — mismo patrón que 0001.
-- ---------------------------------------------------------------------------
ALTER TABLE data_connections ENABLE ROW LEVEL SECURITY;
ALTER TABLE data_queries     ENABLE ROW LEVEL SECURITY;
ALTER TABLE data_connections FORCE ROW LEVEL SECURITY;
ALTER TABLE data_queries     FORCE ROW LEVEL SECURITY;

CREATE POLICY data_connections_tenant_isolation ON data_connections
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY data_queries_tenant_isolation ON data_queries
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

-- ---------------------------------------------------------------------------
-- Grants al rol de aplicación (docs_app). Los GRANT ON ALL TABLES de 0001 solo
-- cubren las tablas existentes en ese momento; las nuevas requieren grant
-- explícito. La membresía docs_app -> current_user ya se concedió en 0001.
-- ---------------------------------------------------------------------------
GRANT SELECT, INSERT, UPDATE, DELETE ON data_connections TO docs_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON data_queries     TO docs_app;
