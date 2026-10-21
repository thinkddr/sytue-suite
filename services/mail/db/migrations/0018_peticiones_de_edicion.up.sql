-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- «Pedir acceso» desde la franja de solo lectura de un buzón compartido o delegado:
-- quien lo tiene en `read` se lo pide a quien lo gestiona (`manage`, o el dueño de un
-- buzón delegado), que lo sube a `send`. Las columnas son las de platform/pkg/peticiones,
-- iguales en todos los servicios; `account_id` es la cuenta del buzón.
CREATE TABLE mailbox_peticiones_edicion (
    account_id uuid        NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    tenant     text        NOT NULL,
    sujeto     text        NOT NULL,
    nombre     text        NOT NULL DEFAULT '',
    correo     text        NOT NULL DEFAULT '',
    creada     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (account_id, sujeto)
);

ALTER TABLE mailbox_peticiones_edicion ENABLE ROW LEVEL SECURITY;
ALTER TABLE mailbox_peticiones_edicion FORCE ROW LEVEL SECURITY;

CREATE POLICY mailbox_peticiones_edicion_tenant_isolation ON mailbox_peticiones_edicion
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

-- Fusión de organizaciones (0010 solo cubrió las tablas que ya existían).
CREATE POLICY mailbox_peticiones_edicion_platform_merge ON mailbox_peticiones_edicion
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
