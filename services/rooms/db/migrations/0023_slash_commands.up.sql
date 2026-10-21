-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- SLASH COMMANDS de terceros: «/desplegar prod» ejecuta código de otro y contesta.
--
-- El registro del CLIENTE (apps/frontend/apps/rooms/src/lib/commands.ts) se escribió
-- desde el principio para admitir comandos de servidor; lo que faltaba era esta mitad.
-- Sin ella, un bot solo puede hablar cuando alguien lo menciona: no hay forma de pedirle
-- algo con un verbo.
--
-- El payload y la firma son los de Slack (form-urlencoded + X-Slack-Signature), así que
-- un comando ya escrito para Slack funciona pegando su URL. Ver internal/compat.
CREATE TABLE slash_commands (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant      text        NOT NULL,
    -- Nombre SIN la barra, en minúsculas. Único por organización: dos comandos con el
    -- mismo nombre harían que cuál se ejecuta dependiera del orden de la consulta.
    name        text        NOT NULL,
    description text        NOT NULL DEFAULT '',
    usage_hint  text        NOT NULL DEFAULT '',
    request_url text        NOT NULL,
    -- Token de verificación, EN CLARO: viaja en cada ejecución (igual que en los
    -- salientes) y por eso no prueba el origen; para eso está la firma.
    token          text NOT NULL,
    -- Secreto de firma. NUNCA sale: es lo que hace que X-Slack-Signature signifique algo.
    signing_secret text NOT NULL,
    created_by  text        NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT slash_commands_name_shape CHECK (name ~ '^[a-z][a-z0-9_-]{0,31}$'),
    CONSTRAINT slash_commands_url_not_blank CHECK (length(request_url) > 0)
);

-- Resolución del comando al ejecutarlo: por (tenant, nombre), que es la clave real.
CREATE UNIQUE INDEX slash_commands_by_name ON slash_commands (tenant, name);

ALTER TABLE slash_commands ENABLE ROW LEVEL SECURITY;
ALTER TABLE slash_commands FORCE ROW LEVEL SECURITY;

CREATE POLICY slash_commands_tenant_isolation ON slash_commands
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
