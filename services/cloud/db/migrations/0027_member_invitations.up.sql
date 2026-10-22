-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Invitaciones pendientes de Sytue Cloud. Sytue ID conserva el token firmado,
-- de un solo uso, y materializa la cuenta/membresía de identidad; Cloud solo
-- guarda el espejo mínimo necesario para aplicar su rol al `sub` real cuando el
-- invitado inicia sesión. Una fila pendiente NO cuenta como puesto facturable.
CREATE TABLE member_invitations (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant      text        NOT NULL,
    email       text        NOT NULL,
    role        text        NOT NULL CHECK (role IN ('owner', 'admin', 'member', 'billing')),
    invited_by  text        NOT NULL DEFAULT '',
    expires_at  timestamptz NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

-- Reenviar sustituye la invitación anterior de ese correo en esa organización.
CREATE UNIQUE INDEX member_invitations_tenant_email_idx
    ON member_invitations (tenant, lower(email));
CREATE INDEX member_invitations_expiry_idx ON member_invitations (expires_at);

ALTER TABLE member_invitations ENABLE ROW LEVEL SECURITY;
ALTER TABLE member_invitations FORCE ROW LEVEL SECURITY;
CREATE POLICY member_invitations_tenant_isolation ON member_invitations
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
