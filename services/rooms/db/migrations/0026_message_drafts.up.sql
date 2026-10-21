-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- BORRADORES EN SERVIDOR (paridad §4: «Borradores solo en localStorage»).
--
-- Hasta aquí el texto escrito y no enviado vivía SOLO en el localStorage del
-- navegador (apps/frontend/apps/rooms/src/lib/drafts.ts): lo redactado en el
-- portátil no existía en el móvil y un borrador olvidado era invisible. Esta tabla
-- lo convierte en estado del servidor con la MISMA clave que ya usaba el cliente:
-- un borrador por (usuario, canal, hilo).
--
-- parent_id NULL = borrador del composer del CANAL; con valor = del composer del
-- HILO cuya raíz es ese mensaje. El único índice es por EXPRESIÓN sobre COALESCE
-- porque en un UNIQUE normal dos NULL no colisionan: sin ese COALESCE cada pulsación
-- insertaría un borrador de canal nuevo en vez de pisar el anterior.
--
-- Es estado PER-USUARIO y de tamaño acotado (un borrador por ámbito abierto), así
-- que no lleva keyset ni índice extra: la lista del usuario se lee por el prefijo
-- (tenant, subject) del mismo índice único.

CREATE TABLE message_drafts (
    tenant     text        NOT NULL,
    subject    text        NOT NULL,
    channel_id uuid        NOT NULL REFERENCES channels (id) ON DELETE CASCADE,
    parent_id  uuid        REFERENCES messages (id) ON DELETE CASCADE,
    body       text        NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT message_drafts_body_not_blank CHECK (length(body) > 0)
);

CREATE UNIQUE INDEX message_drafts_scope ON message_drafts
    (tenant, subject, channel_id, COALESCE(parent_id, '00000000-0000-0000-0000-000000000000'::uuid));

-- RLS deny-by-default por tenant (mismo patrón que el resto del esquema). FORCE
-- porque el rol de la aplicación es el PROPIETARIO de la tabla: sin él la política
-- no se evalúa y los borradores se verían entre organizaciones.
ALTER TABLE message_drafts ENABLE ROW LEVEL SECURITY;
ALTER TABLE message_drafts FORCE ROW LEVEL SECURITY;
CREATE POLICY message_drafts_tenant_isolation ON message_drafts
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
