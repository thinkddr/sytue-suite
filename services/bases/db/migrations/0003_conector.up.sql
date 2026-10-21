-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Sytue Bases, conector de SOLO LECTURA: una conexión da a un consumidor (Calcus) permiso
-- para LEER una tabla —o una de sus vistas— con un token acotado, con caducidad y
-- revocable. El token NUNCA se guarda: solo su SHA-256.
--
-- La procedencia (qué base, tabla y vista, quién lo concedió, cuándo y qué versión) sale
-- de esta fila en cada lectura; `version` sube al rotar el token. Cada lectura queda en
-- `base_audit` (acción `connector.read`).

CREATE TABLE base_connections (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant       text NOT NULL,
    base_id      uuid NOT NULL,
    table_id     uuid NOT NULL,
    -- NULL = la tabla entera; con valor, solo lo que esa vista filtra y ordena.
    view_id      uuid,
    name         text NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
    consumer     text NOT NULL DEFAULT 'calcus' CHECK (consumer IN ('calcus')),
    token_hash   text NOT NULL UNIQUE CHECK (token_hash ~ '^[0-9a-f]{64}$'),
    version      integer NOT NULL DEFAULT 1 CHECK (version >= 1),
    -- Siempre caduca: un token de lectura sin fecha es un acceso que nadie recuerda retirar.
    expires_at   timestamptz NOT NULL,
    revoked_at   timestamptz,
    revoked_by   text,
    reads        bigint NOT NULL DEFAULT 0,
    last_read_at timestamptz,
    created_by   text NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant, id),
    FOREIGN KEY (tenant, table_id, base_id) REFERENCES base_tables (tenant, id, base_id) ON DELETE CASCADE ON UPDATE CASCADE,
    FOREIGN KEY (tenant, view_id) REFERENCES base_views (tenant, id) ON DELETE CASCADE ON UPDATE CASCADE
);
CREATE INDEX base_connections_por_base ON base_connections (tenant, base_id, created_at DESC);

ALTER TABLE base_connections ENABLE ROW LEVEL SECURITY;
ALTER TABLE base_connections FORCE ROW LEVEL SECURITY;
CREATE POLICY base_connections_tenant_isolation ON base_connections
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY base_connections_platform_merge ON base_connections
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
-- Solo lectura, para resolver un token sin saber de qué organización es.
CREATE POLICY base_connections_publico ON base_connections FOR SELECT
    USING (current_setting('app.bases_publico', true) = 'on');
