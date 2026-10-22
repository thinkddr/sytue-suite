-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- APNs «SOLO DESPERTAR» (app nativa de iOS): tokens de dispositivo a los que mandar
-- un aviso VACÍO. Apple es el único camino para despertar una app de iOS, y no tiene
-- región europea: por eso lo que sale hacia allí es lo mínimo que existe —el token
-- del dispositivo y el momento—, NUNCA el título, el cuerpo, el autor ni el canal.
-- El push lleva solo `content-available`; la app despierta y pide el contenido a
-- nuestros servidores, que sí están en la UE. Ver `service/apns.go`.
--
-- Un usuario puede tener varios dispositivos: la clave es (tenant, token) —el token
-- identifica al dispositivo—; re-registrarlo hace UPSERT (el mismo aparato puede
-- cambiar de dueño al reinstalar). El índice por (tenant, subject) cubre la consulta
-- del enviador sin escaneo, igual que en push_subscriptions (0016).
--
-- Seguridad: RLS deny-by-default por tenant. El token NO es público: quien lo tenga
-- puede mandar avisos a ese aparato, así que se aísla como un secreto.

CREATE TABLE push_apns (
    tenant     text        NOT NULL,
    subject    text        NOT NULL,
    token      text        NOT NULL,   -- token de dispositivo APNs (hex)
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, token)
);

CREATE INDEX push_apns_subject_idx ON push_apns (tenant, subject);

ALTER TABLE push_apns ENABLE ROW LEVEL SECURITY;
ALTER TABLE push_apns FORCE ROW LEVEL SECURITY;

CREATE POLICY push_apns_tenant_isolation ON push_apns
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY push_apns_platform_merge ON push_apns
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
