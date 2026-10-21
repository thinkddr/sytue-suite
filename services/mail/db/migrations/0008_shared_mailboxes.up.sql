-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Buzones compartidos + delegación de buzón.
--
-- Un buzón COMPARTIDO (soporte@, ventas@) es una `accounts` de la ORG, no de una
-- persona: se marca con kind='shared' y un subject sintético ("shared:<addr>").
-- Reúsa TODO el modelo existente: buzones de sistema, entrega entrante
-- (LookupAddress ya resuelve su dirección → cuenta), mensajes, hilos, Enviados,
-- firma DKIM. La DELEGACIÓN (asistente ↔ directivo) es el mismo modelo de ACL
-- sobre la cuenta PERSONAL de otro (kind='user').
--
-- El acceso lo concede `mailbox_acl`: un grantee (sub de Sytue ID) con un nivel
-- de permiso sobre una cuenta. Escalera deny-by-default: read < send < manage.
-- Sin fila de ACL vigente NO hay acceso (lo comprueba el servicio en cada acceso,
-- además del aislamiento por tenant de RLS).

-- kind distingue la cuenta de persona del buzón compartido de organización.
ALTER TABLE accounts
    ADD COLUMN kind text NOT NULL DEFAULT 'user'
        CHECK (kind IN ('user', 'shared'));

-- ACL de acceso a buzón (compartido o delegado).
CREATE TABLE mailbox_acl (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant          text        NOT NULL,
    account_id      uuid        NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    grantee_subject text        NOT NULL,                 -- sub de Sytue ID del delegado
    permission      text        NOT NULL DEFAULT 'read'
                    CHECK (permission IN ('read', 'send', 'manage')),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

-- Un grantee tiene a lo sumo una fila por cuenta (re-conceder ACTUALIZA el nivel).
CREATE UNIQUE INDEX mailbox_acl_unique ON mailbox_acl (account_id, grantee_subject);
-- «Buzones a los que YO accedo» sin N+1: listar por (tenant, grantee).
CREATE INDEX mailbox_acl_grantee_idx ON mailbox_acl (tenant, grantee_subject);

-- RLS deny-by-default por tenant (CONVENTIONS §10), como el resto del esquema. NO
-- lleva política de mantenimiento: el ACL solo se lee/escribe en contexto de tenant
-- (API JMAP); la delegación y los buzones compartidos NUNCA cruzan tenant.
ALTER TABLE mailbox_acl ENABLE ROW LEVEL SECURITY;
ALTER TABLE mailbox_acl FORCE ROW LEVEL SECURITY;
CREATE POLICY mailbox_acl_tenant_isolation ON mailbox_acl
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
