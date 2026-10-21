-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- WEB PUSH (Ola 18 · PWA móvil): suscripciones Web Push (RFC 8291) por usuario para
-- avisar de @menciones y DMs cuando la app está CERRADA (el Service Worker recibe el
-- push y muestra la notificación). Cada fila es una suscripción del navegador/dispositivo
-- del usuario: su `endpoint` (URL del servicio de push del fabricante) y las claves
-- públicas del cliente (p256dh, auth) con las que el servidor cifra el payload.
--
-- Un usuario puede tener VARIAS suscripciones (varios dispositivos/navegadores): la clave
-- es (tenant, endpoint) —el endpoint identifica unívocamente una suscripción—; re-suscribir
-- el mismo endpoint hace UPSERT (actualiza subject/claves). El índice por (tenant, subject)
-- cubre la consulta del enviador (dame las suscripciones de ese destinatario) sin escaneo.
--
-- Escala: el enviador lee por (tenant, subject) —indexado—; los destinatarios de un mensaje
-- son sus @mencionados + (si es DM) el otro miembro, un conjunto acotado. Nada de fan-out
-- por miembro de canal grande (eso iría por cola, fuera de esta ola).
--
-- Seguridad: RLS deny-by-default por tenant (mismo patrón que user_notify_prefs / read_state).
-- Las claves son PÚBLICAS del cliente (no secretas), pero se aíslan por tenant igualmente.

CREATE TABLE push_subscriptions (
    tenant     text        NOT NULL,
    subject    text        NOT NULL,
    endpoint   text        NOT NULL,   -- URL del servicio de push (https)
    p256dh     text        NOT NULL,   -- base64url, clave pública P-256 del cliente
    auth_key   text        NOT NULL,   -- base64url, secreto de autenticación (16 bytes)
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, endpoint)
);

-- Lookup del enviador: todas las suscripciones de un destinatario.
CREATE INDEX push_subscriptions_subject_idx ON push_subscriptions (tenant, subject);

ALTER TABLE push_subscriptions ENABLE ROW LEVEL SECURITY;
ALTER TABLE push_subscriptions FORCE ROW LEVEL SECURITY;
CREATE POLICY push_subscriptions_tenant_isolation ON push_subscriptions
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
